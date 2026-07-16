package dockerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestContainerListMapsAndSortsRedactedSummaries(t *testing.T) {
	socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/version":
			writeJSONResponse(t, writer, map[string]string{"Version": "29.1.0", "ApiVersion": "1.55", "Os": "linux", "Arch": "amd64"})
		case "/v1.55/containers/json":
			if request.URL.Query().Get("all") != "1" || request.URL.Query().Get("limit") != "2" {
				t.Errorf("unexpected list query: %s", request.URL.RawQuery)
			}
			writeJSONResponse(t, writer, []map[string]any{
				{
					"Id":      "bbbb",
					"Names":   []string{"/zeta"},
					"Image":   "redis:latest",
					"ImageID": "sha256:redis",
					"Created": int64(1_700_000_100),
					"State":   "exited",
					"Status":  "Exited (1) 2 minutes ago",
					"Labels":  map[string]string{"secret": "do-not-return"},
					"Ports": []map[string]any{
						{"PrivatePort": 6379, "Type": "tcp"},
					},
					"NetworkSettings": map[string]any{"Networks": map[string]any{
						"z-net": map[string]any{"IPAddress": "172.20.0.3", "Gateway": "172.20.0.1"},
					}},
				},
				{
					"Id":      "aaaa",
					"Names":   []string{"/alpha", "/alpha-alias"},
					"Image":   "nginx:stable",
					"ImageID": "sha256:nginx",
					"Created": int64(1_700_000_000),
					"State":   "running",
					"Status":  "Up 4 minutes (healthy)",
					"Health":  map[string]any{"Status": "healthy", "FailingStreak": 0},
					"Ports": []map[string]any{
						{"IP": "0.0.0.0", "PrivatePort": 443, "PublicPort": 8443, "Type": "tcp"},
						{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
					},
					"NetworkSettings": map[string]any{"Networks": map[string]any{
						"front": map[string]any{"IPAddress": "172.19.0.2", "GlobalIPv6Address": "2001:db8::2", "Gateway": "172.19.0.1"},
						"back":  map[string]any{"IPAddress": "172.18.0.2", "Gateway": "172.18.0.1"},
					}},
				},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	containers, err := client.ContainerList(context.Background(), ContainerListOptions{All: true, Limit: 2})
	if err != nil {
		t.Fatalf("container list: %v", err)
	}
	if len(containers) != 2 || containers[0].Names[0] != "alpha" || containers[1].Names[0] != "zeta" {
		t.Fatalf("containers were not sorted: %#v", containers)
	}
	if containers[0].CreatedAt != "2023-11-14T22:13:20Z" {
		t.Fatalf("unexpected creation time: %s", containers[0].CreatedAt)
	}
	if containers[0].Health == nil || containers[0].Health.Status != "healthy" {
		t.Fatalf("missing health summary: %#v", containers[0].Health)
	}
	if len(containers[0].Ports) != 2 || containers[0].Ports[0].PrivatePort != 80 {
		t.Fatalf("ports were not sorted: %#v", containers[0].Ports)
	}
	if len(containers[0].Networks) != 2 || containers[0].Networks[0].Name != "back" {
		t.Fatalf("networks were not sorted: %#v", containers[0].Networks)
	}
	payload, err := json.Marshal(containers)
	if err != nil {
		t.Fatalf("marshal containers: %v", err)
	}
	if strings.Contains(string(payload), "do-not-return") || strings.Contains(string(payload), "secret") {
		t.Fatalf("raw labels leaked into output: %s", payload)
	}
}

func TestContainerInspectReturnsRedactedDeterministicView(t *testing.T) {
	socketPath := startUnixHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/version":
			writeJSONResponse(t, writer, map[string]string{"Version": "29.1.0", "ApiVersion": "1.55", "Os": "linux", "Arch": "amd64"})
		case "/v1.55/containers/web/json":
			writeJSONResponse(t, writer, map[string]any{
				"Id":           "0123456789abcdef",
				"Name":         "/web",
				"Created":      "2026-07-16T10:00:00Z",
				"Image":        "sha256:image",
				"Driver":       "overlay2",
				"Platform":     "linux",
				"RestartCount": 3,
				"Path":         "/bin/sh",
				"Args":         []string{"-c", "echo super-secret-command"},
				"State": map[string]any{
					"Status":     "running",
					"Running":    true,
					"Pid":        4242,
					"ExitCode":   0,
					"StartedAt":  "2026-07-16T10:01:00Z",
					"FinishedAt": "0001-01-01T00:00:00Z",
					"Health": map[string]any{
						"Status":        "unhealthy",
						"FailingStreak": 4,
						"Log": []map[string]any{{"Output": "database password leaked here"}},
					},
				},
				"HostConfig": map[string]any{
					"NetworkMode":     "bridge",
					"RestartPolicy":   map[string]any{"Name": "unless-stopped", "MaximumRetryCount": 0},
					"AutoRemove":      false,
					"Privileged":      false,
					"ReadonlyRootfs":  true,
					"Memory":          int64(512 << 20),
					"NanoCpus":        int64(2_000_000_000),
					"PidsLimit":       int64(256),
				},
				"Config": map[string]any{
					"Image":      "example/web:stable",
					"User":       "1000:1000",
					"WorkingDir": "/app",
					"StopSignal": "SIGTERM",
					"Env":        []string{"DATABASE_PASSWORD=super-secret"},
					"Cmd":        []string{"run", "--token", "super-secret-token"},
					"Labels":     map[string]string{"api-key": "super-secret-key"},
				},
				"NetworkSettings": map[string]any{
					"Ports": map[string]any{
						"443/tcp": []map[string]string{{"HostIp": "0.0.0.0", "HostPort": "8443"}},
						"80/tcp":  []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "8080"}},
					},
					"Networks": map[string]any{
						"z-net": map[string]string{"IPAddress": "172.20.0.2", "Gateway": "172.20.0.1"},
						"a-net": map[string]string{"IPAddress": "172.18.0.2", "Gateway": "172.18.0.1"},
					},
				},
				"Mounts": []map[string]any{
					{"Type": "bind", "Source": "/host/private/data", "Destination": "/data", "RW": false},
					{"Type": "volume", "Name": "cache", "Source": "/var/lib/docker/volumes/cache", "Destination": "/cache", "RW": true},
				},
				"LogPath": "/var/lib/docker/containers/secret-json.log",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	client, err := New(Config{SocketPath: socketPath, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	container, err := client.ContainerInspect(context.Background(), "web")
	if err != nil {
		t.Fatalf("container inspect: %v", err)
	}
	if container.Name != "web" || container.State.Health == nil || container.State.Health.FailingStreak != 4 {
		t.Fatalf("unexpected inspect result: %#v", container)
	}
	if !container.Runtime.ReadonlyRootFS || container.Resources.PIDsLimit != 256 {
		t.Fatalf("missing runtime safeguards: %#v %#v", container.Runtime, container.Resources)
	}
	if len(container.Networks) != 2 || container.Networks[0].Name != "a-net" {
		t.Fatalf("networks were not sorted: %#v", container.Networks)
	}
	if len(container.Ports) != 2 || container.Ports[0].ContainerPort != "443/tcp" {
		t.Fatalf("ports were not sorted: %#v", container.Ports)
	}
	if len(container.Mounts) != 2 || container.Mounts[0].Destination != "/cache" {
		t.Fatalf("mounts were not sorted: %#v", container.Mounts)
	}
	payload, err := json.Marshal(container)
	if err != nil {
		t.Fatalf("marshal container: %v", err)
	}
	for _, secret := range []string{
		"super-secret-command",
		"database password leaked here",
		"super-secret",
		"/host/private/data",
		"/var/lib/docker/volumes/cache",
		"secret-json.log",
	} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("sensitive value %q leaked into output: %s", secret, payload)
		}
	}
}
