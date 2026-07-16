package main

import "testing"

func TestBuildRegistryIncludesTLSInspect(t *testing.T) {
	t.Setenv("OPSPILOT_HTTP_ALLOW_PRIVATE", "false")
	t.Setenv("OPSPILOT_TLS_ALLOW_PRIVATE", "false")
	registry, err := buildRegistry()
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}

	definitions := registry.Definitions()
	want := []string{"dns_lookup", "http_probe", "tls_inspect"}
	if len(definitions) != len(want) {
		t.Fatalf("unexpected tool count: %#v", definitions)
	}
	for index, name := range want {
		if definitions[index].Name != name {
			t.Fatalf("tool %d = %q, want %q", index, definitions[index].Name, name)
		}
	}
}
