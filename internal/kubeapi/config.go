package kubeapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	defaultNamespace = "default"
	defaultTimeout   = 10 * time.Second
	defaultQPS       = 5
	defaultBurst     = 10
)

type Config struct {
	KubeconfigPath string
	Context        string
	Timeout        time.Duration
	QPS            float32
	Burst          int
}

func loadRESTConfig(config Config) (*rest.Config, string, error) {
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.QPS <= 0 {
		config.QPS = defaultQPS
	}
	if config.Burst <= 0 {
		config.Burst = defaultBurst
	}

	if strings.TrimSpace(config.KubeconfigPath) == "" {
		if inCluster, err := rest.InClusterConfig(); err == nil {
			namespace := readInClusterNamespace()
			if err := hardenRESTConfig(inCluster, config); err != nil {
				return nil, "", err
			}
			return inCluster, namespace, nil
		} else if !errors.Is(err, rest.ErrNotInCluster) {
			return nil, "", errors.New("kubernetes_config_invalid: unable to load in-cluster configuration")
		}
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.Warner = func(error) {}
	if path := strings.TrimSpace(config.KubeconfigPath); path != "" {
		if !filepath.IsAbs(path) {
			return nil, "", errors.New("kubernetes_config_invalid: kubeconfig path must be absolute")
		}
		rules.ExplicitPath = filepath.Clean(path)
	}

	rawConfig, err := rules.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", errors.New("kubernetes_config_not_found: kubeconfig file was not found")
		}
		return nil, "", errors.New("kubernetes_config_invalid: kubeconfig could not be loaded")
	}
	if rawConfig == nil || len(rawConfig.Clusters) == 0 || len(rawConfig.Contexts) == 0 {
		return nil, "", errors.New("kubernetes_config_not_found: no usable Kubernetes configuration was found")
	}
	if err := validateRawConfig(rawConfig); err != nil {
		return nil, "", err
	}

	contextName := strings.TrimSpace(config.Context)
	if contextName == "" {
		contextName = rawConfig.CurrentContext
	}
	if contextName == "" {
		return nil, "", errors.New("kubernetes_config_invalid: kubeconfig has no current context")
	}
	clientConfig := clientcmd.NewNonInteractiveClientConfig(
		*rawConfig,
		contextName,
		&clientcmd.ConfigOverrides{},
		rules,
	)
	namespace, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, "", errors.New("kubernetes_config_invalid: current namespace could not be resolved")
	}
	if strings.TrimSpace(namespace) == "" {
		namespace = defaultNamespace
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, "", errors.New("kubernetes_config_invalid: REST configuration could not be created")
	}
	if err := hardenRESTConfig(restConfig, config); err != nil {
		return nil, "", err
	}
	return restConfig, namespace, nil
}

func validateRawConfig(config *clientcmdapi.Config) error {
	for _, cluster := range config.Clusters {
		if cluster == nil {
			continue
		}
		serverURL, err := url.Parse(strings.TrimSpace(cluster.Server))
		if err != nil || serverURL.Scheme != "https" || serverURL.Host == "" || serverURL.User != nil {
			return errors.New("kubernetes_config_unsafe: API server must be an HTTPS URL without user information")
		}
		if cluster.InsecureSkipTLSVerify {
			return errors.New("kubernetes_config_unsafe: insecure-skip-tls-verify is not allowed")
		}
		if strings.TrimSpace(cluster.ProxyURL) != "" {
			return errors.New("kubernetes_config_unsafe: kubeconfig proxy URLs are not allowed")
		}
	}
	for _, authInfo := range config.AuthInfos {
		if authInfo == nil {
			continue
		}
		if authInfo.Exec != nil {
			return errors.New("kubernetes_config_unsafe: exec credential plugins are not allowed")
		}
		if authInfo.AuthProvider != nil {
			return errors.New("kubernetes_config_unsafe: auth-provider plugins are not allowed")
		}
		if authInfo.Impersonate != "" || authInfo.ImpersonateUID != "" || len(authInfo.ImpersonateGroups) > 0 || len(authInfo.ImpersonateUserExtra) > 0 {
			return errors.New("kubernetes_config_unsafe: user impersonation is not allowed")
		}
	}
	return nil
}

func hardenRESTConfig(restConfig *rest.Config, config Config) error {
	if restConfig == nil {
		return errors.New("kubernetes_config_invalid: REST configuration is nil")
	}
	serverURL, err := url.Parse(strings.TrimSpace(restConfig.Host))
	if err != nil || serverURL.Scheme != "https" || serverURL.Host == "" || serverURL.User != nil {
		return errors.New("kubernetes_config_unsafe: API server must be an HTTPS URL without user information")
	}
	if restConfig.TLSClientConfig.Insecure {
		return errors.New("kubernetes_config_unsafe: insecure TLS verification is not allowed")
	}
	if restConfig.Impersonate.UserName != "" || restConfig.Impersonate.UID != "" || len(restConfig.Impersonate.Groups) > 0 || len(restConfig.Impersonate.Extra) > 0 {
		return errors.New("kubernetes_config_unsafe: user impersonation is not allowed")
	}

	restConfig.Timeout = config.Timeout
	restConfig.QPS = config.QPS
	restConfig.Burst = config.Burst
	restConfig.UserAgent = "opspilot/kubernetes-readonly"
	restConfig.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	return nil
}

func readInClusterNamespace() string {
	payload, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return defaultNamespace
	}
	namespace := strings.TrimSpace(string(payload))
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

func configSummary(config Config) string {
	if strings.TrimSpace(config.KubeconfigPath) != "" {
		return fmt.Sprintf("kubeconfig context %q", strings.TrimSpace(config.Context))
	}
	return "in-cluster or default kubeconfig"
}
