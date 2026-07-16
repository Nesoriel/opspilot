package main

import "testing"

func TestBuildRegistryIncludesReadOnlyDiagnostics(t *testing.T) {
	t.Setenv("OPSPILOT_HTTP_ALLOW_PRIVATE", "false")
	t.Setenv("OPSPILOT_TLS_ALLOW_PRIVATE", "false")
	t.Setenv("OPSPILOT_DOCKER_SOCKET", "")
	t.Setenv("OPSPILOT_KUBECONFIG", "/definitely/not/loaded/during-registry-build")
	t.Setenv("OPSPILOT_PROMETHEUS_URL", "")
	registry, err := buildRegistry()
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}

	definitions := registry.Definitions()
	want := []string{
		"dns_lookup",
		"docker_container_inspect",
		"docker_container_list",
		"docker_engine_info",
		"http_probe",
		"kubernetes_cluster_info",
		"kubernetes_pod_inspect",
		"kubernetes_pod_list",
		"prometheus_metric_snapshot",
		"prometheus_server_info",
		"prometheus_target_list",
		"tls_inspect",
	}
	if len(definitions) != len(want) {
		t.Fatalf("unexpected tool count: %#v", definitions)
	}
	for index, name := range want {
		if definitions[index].Name != name {
			t.Fatalf("tool %d = %q, want %q", index, definitions[index].Name, name)
		}
	}
}

func TestBuildRegistryRejectsRemoteDockerTargets(t *testing.T) {
	t.Setenv("OPSPILOT_DOCKER_SOCKET", "tcp://127.0.0.1:2375")
	if _, err := buildRegistry(); err == nil {
		t.Fatal("expected remote Docker target to be rejected")
	}
}
