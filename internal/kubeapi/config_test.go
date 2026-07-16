package kubeapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestLoadRESTConfigAcceptsSafeKubeconfig(t *testing.T) {
	path := writeKubeconfig(t, func(config *clientcmdapi.Config) {
		config.Contexts["safe"].Namespace = "operations"
	})
	restConfig, namespace, err := loadRESTConfig(Config{
		KubeconfigPath: path,
		Context:        "safe",
		Timeout:        3 * time.Second,
		QPS:            7,
		Burst:          12,
	})
	if err != nil {
		t.Fatalf("load REST config: %v", err)
	}
	if namespace != "operations" {
		t.Fatalf("namespace = %q, want operations", namespace)
	}
	if restConfig.Host != "https://kubernetes.example.test:6443" {
		t.Fatalf("unexpected host: %q", restConfig.Host)
	}
	if restConfig.Timeout != 3*time.Second || restConfig.QPS != 7 || restConfig.Burst != 12 {
		t.Fatalf("unexpected limits: timeout=%v qps=%v burst=%d", restConfig.Timeout, restConfig.QPS, restConfig.Burst)
	}
	if restConfig.UserAgent != "opspilot/kubernetes-readonly" {
		t.Fatalf("unexpected user agent: %q", restConfig.UserAgent)
	}
	proxy, err := restConfig.Proxy(&http.Request{})
	if err != nil || proxy != nil {
		t.Fatalf("proxy was not disabled: %v %v", proxy, err)
	}
}

func TestLoadRESTConfigRejectsUnsafeKubeconfigFeatures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*clientcmdapi.Config)
		code   string
	}{
		{
			name: "HTTP API server",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["safe"].Server = "http://127.0.0.1:8080"
			},
			code: "kubernetes_config_unsafe",
		},
		{
			name: "insecure TLS",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["safe"].InsecureSkipTLSVerify = true
			},
			code: "insecure-skip-tls-verify",
		},
		{
			name: "proxy URL",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["safe"].ProxyURL = "socks5://127.0.0.1:1080"
			},
			code: "proxy URLs",
		},
		{
			name: "exec credentials",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["safe"].Exec = &clientcmdapi.ExecConfig{Command: "credential-helper"}
			},
			code: "exec credential plugins",
		},
		{
			name: "auth provider",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["safe"].AuthProvider = &clientcmdapi.AuthProviderConfig{Name: "oidc"}
			},
			code: "auth-provider plugins",
		},
		{
			name: "impersonation",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["safe"].Impersonate = "cluster-admin"
			},
			code: "impersonation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeKubeconfig(t, test.mutate)
			_, _, err := loadRESTConfig(Config{KubeconfigPath: path, Context: "safe"})
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHardenRESTConfigRejectsUnsafeResolvedConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config *rest.Config
	}{
		{name: "HTTP host", config: &rest.Config{Host: "http://127.0.0.1:8080"}},
		{name: "insecure TLS", config: &rest.Config{Host: "https://kubernetes.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}},
		{name: "impersonation", config: &rest.Config{Host: "https://kubernetes.example.test", Impersonate: rest.ImpersonationConfig{UserName: "cluster-admin"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := hardenRESTConfig(test.config, Config{Timeout: time.Second, QPS: 1, Burst: 1}); err == nil {
				t.Fatal("expected unsafe resolved configuration to be rejected")
			}
		})
	}
}

func TestLoadRESTConfigRejectsRelativeAndMissingPaths(t *testing.T) {
	if _, _, err := loadRESTConfig(Config{KubeconfigPath: "relative/config"}); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("unexpected relative path error: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-kubeconfig")
	if _, _, err := loadRESTConfig(Config{KubeconfigPath: missing}); err == nil || !strings.Contains(err.Error(), "kubernetes_config_") {
		t.Fatalf("unexpected missing path error: %v", err)
	}
}

func TestValidateRawConfigChecksUnusedEntries(t *testing.T) {
	config := safeRawConfig()
	config.AuthInfos["unused"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "unexpected"}}
	if err := validateRawConfig(config); err == nil || !strings.Contains(err.Error(), "exec credential plugins") {
		t.Fatalf("unused unsafe auth entry was accepted: %v", err)
	}
}

func writeKubeconfig(t *testing.T, mutate func(*clientcmdapi.Config)) string {
	t.Helper()
	config := safeRawConfig()
	if mutate != nil {
		mutate(config)
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

func safeRawConfig() *clientcmdapi.Config {
	return &clientcmdapi.Config{
		CurrentContext: "safe",
		Clusters: map[string]*clientcmdapi.Cluster{
			"safe": {Server: "https://kubernetes.example.test:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"safe": {Token: "test-token"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"safe": {Cluster: "safe", AuthInfo: "safe", Namespace: defaultNamespace},
		},
	}
}

func TestReadInClusterNamespaceFallsBack(t *testing.T) {
	if namespace := readInClusterNamespace(); strings.TrimSpace(namespace) == "" {
		t.Fatal("namespace fallback was empty")
	}
}
