# OpsPilot

OpsPilot is a code-first, safety-oriented operations agent implemented in Go. Its core runtime stays provider-neutral while adapters integrate with the Volcengine AI ecosystem.

> Status: early development. The project includes a bounded Agent Runtime, an Ark Responses API adapter, an MCP stdio server, privacy-safe runtime events, optional OpenTelemetry tracing, and machine-readable, read-only network, Docker, and Kubernetes diagnostics.

## Design goals

- Go-first runtime suitable for containers, Kubernetes, OpenClaw, Hermes, and other agents.
- Volcengine Ark, VikingDB, AgentKit, and VKE integrations behind replaceable interfaces.
- Read-only diagnostics by default; state-changing operations require explicit policy and approval.
- Structured JSON inputs and outputs for reliable agent-to-tool communication.
- Timeouts, step limits, tests, and observable execution rather than prompt-only safety.

## Current capabilities

- Provider-neutral agent loop with bounded steps and structured tool errors.
- Volcengine Ark Responses API adapter through CloudWeGo Eino `agenticark`.
- Official MCP Go SDK stdio server backed by the same tool registry as the CLI and Agent Runtime.
- Strongly defined tool registry with duplicate and schema validation.
- Read-only `dns_lookup`, SSRF-aware `http_probe`, and certificate-aware `tls_inspect` tools.
- Read-only Docker Engine, container-list, and redacted container-inspect diagnostics over a local Unix socket.
- Read-only Kubernetes server, node, Pod-list, and redacted Pod-inspect diagnostics through client-go v0.36.2.
- Shared network guard that resolves and validates every dial target before connecting.
- Machine-readable CLI intended for agents and automation.
- JSONL lifecycle events with run IDs, step numbers, durations, and sanitized error classes.
- Optional OTLP/HTTP traces for Agent runs, model calls, and tool executions.
- Configuration validation and provider-error secret redaction.

## Toolchain

OpsPilot tracks the latest stable Go toolchain. The current baseline is Go 1.26.5.

```bash
go version
go mod tidy
go test ./...
go build ./cmd/opspilot
```

## Configure Ark

Copy `.env.example` into your preferred secret-management workflow and provide at least:

```bash
export ARK_MODEL_ID='ep-xxxxxxxxxxxxxxxx'
export ARK_API_KEY='your-api-key'
```

`ARK_THINKING` accepts `auto`, `enabled`, or `disabled`. Credentials are read from the environment and must not be committed.

## Run the agent

```bash
go run ./cmd/opspilot agent run \
  'Check Kubernetes node readiness and identify unhealthy or restarting Pods in the operations namespace.'
```

The command writes the final structured result to stdout. The Ark model can select from the registered read-only tools.

### Stream lifecycle events

```bash
go run ./cmd/opspilot agent run --events=jsonl \
  'Inspect example.com.' \
  2>events.jsonl
```

JSONL events are written to stderr, so agents can consume the final result from stdout independently. Events intentionally omit prompts, tool arguments, tool results, credentials, and raw provider errors.

### Export OpenTelemetry traces

```bash
export OPSPILOT_OTEL_ENABLED=true
export OTEL_SERVICE_NAME=opspilot
export OTEL_EXPORTER_OTLP_ENDPOINT='http://localhost:4318'

go run ./cmd/opspilot agent run 'Inspect example.com.'
```

The OTLP/HTTP exporter follows standard OpenTelemetry environment variables. Telemetry initialization or shutdown failures do not fail the diagnostic run.

## Run as an MCP server

Build the binary and expose the registered read-only tools over stdio:

```bash
go build -o ./bin/opspilot ./cmd/opspilot
./bin/opspilot mcp stdio
```

A typical MCP client configuration is:

```json
{
  "mcpServers": {
    "opspilot": {
      "command": "/absolute/path/to/opspilot",
      "args": ["mcp", "stdio"],
      "env": {
        "OPSPILOT_HTTP_ALLOW_PRIVATE": "false",
        "OPSPILOT_TLS_ALLOW_PRIVATE": "false",
        "OPSPILOT_DOCKER_SOCKET": "/var/run/docker.sock",
        "OPSPILOT_KUBECONFIG": "/absolute/path/to/kubeconfig",
        "OPSPILOT_KUBERNETES_CONTEXT": "production-readonly"
      }
    }
  }
}
```

The server publishes each Registry tool with its existing JSON Schema and explicit read-only/idempotent annotations. Tool results are returned as text and, when the result is a JSON object, as MCP structured content. stdout is reserved exclusively for MCP protocol frames; warnings and failures go to stderr.

## Use the tool runtime directly

```bash
go run ./cmd/opspilot tool list

go run ./cmd/opspilot tool run dns_lookup \
  '{"host":"example.com"}'

go run ./cmd/opspilot tool run http_probe \
  '{"url":"https://example.com"}'

go run ./cmd/opspilot tool run tls_inspect \
  '{"host":"example.com","port":443}'

go run ./cmd/opspilot tool run docker_engine_info '{}'

go run ./cmd/opspilot tool run docker_container_list \
  '{"all":true,"limit":100}'

go run ./cmd/opspilot tool run docker_container_inspect \
  '{"container":"web"}'

go run ./cmd/opspilot tool run kubernetes_cluster_info \
  '{"node_limit":100}'

go run ./cmd/opspilot tool run kubernetes_pod_list \
  '{"namespace":"operations","limit":100}'

go run ./cmd/opspilot tool run kubernetes_pod_inspect \
  '{"namespace":"operations","pod":"web-0","event_limit":50}'
```

### Docker diagnostic boundary

`OPSPILOT_DOCKER_SOCKET` defaults to `/var/run/docker.sock`. It accepts an absolute filesystem path or a `unix:///absolute/path` URI. Remote `tcp://`, `http://`, `https://`, `ssh://`, and relative targets are rejected.

The Docker client negotiates the daemon API through `/version`, then performs bounded GET requests. It does not invoke the Docker CLI and does not expose mutating operations.

`docker_container_inspect` returns a deliberate diagnostic projection rather than raw `docker inspect` output. It omits environment values, commands, arguments, raw labels, health-check output, Docker log paths, bind-mount and volume source paths, and free-text OCI/runtime errors.

Engine warning text is also omitted; only `warning_count` is returned. Container runtime errors are represented by `error_present` without returning the raw text.

Access to a Docker Unix socket is still a privileged host capability. OpsPilot's read-only implementation does not turn the socket itself into a read-only security boundary. Only grant the process access to a trusted local socket, and do not mount that socket into untrusted containers.

### Kubernetes diagnostic boundary

OpsPilot uses the official Kubernetes client-go v0.36.2. Kubernetes configuration is initialized lazily, so missing credentials do not prevent non-Kubernetes tools or the MCP server from starting.

When running outside a cluster, set `OPSPILOT_KUBECONFIG` to an absolute kubeconfig path. `OPSPILOT_KUBERNETES_CONTEXT` optionally selects a context. When running inside Kubernetes without an explicit kubeconfig, OpsPilot uses the mounted ServiceAccount token and CA.

Before constructing a Kubernetes client, OpsPilot rejects kubeconfigs that contain:

- HTTP API servers;
- `insecure-skip-tls-verify`;
- kubeconfig proxy URLs;
- exec credential plugins;
- legacy auth-provider plugins;
- user impersonation.

The model cannot provide a kubeconfig path, API server URL, arbitrary resource type, selector, API path, or HTTP method in tool arguments.

`kubernetes_pod_inspect` returns a deliberate projection rather than a raw Pod object. It omits environment values, commands, arguments, labels, annotations, volume source details, Secret and ConfigMap references, Pod logs, and all free-text condition, container-state, Pod-status, and Event messages. Event output is aggregated by `type` and `reason` only.

Apply the included minimum RBAC objects for an in-cluster deployment:

```bash
kubectl apply -f deploy/kubernetes/opspilot-readonly-rbac.yaml
```

The role grants only:

- GET on `/version`;
- GET/LIST on Nodes and Pods;
- LIST on Events.

It does not grant Secret access or the `pods/log` subresource.

Private, loopback, link-local, multicast, and unspecified HTTP/TLS targets are blocked by default. Set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` or `OPSPILOT_TLS_ALLOW_PRIVATE=true` only in a trusted environment where internal service diagnostics are intended.

## Roadmap

1. MCP client support and richer Agent skill packaging.
2. Prometheus and Loki diagnostics.
3. PostgreSQL task state and VikingDB retrieval.
4. Approval gates, AgentKit/VKE deployment, and production evaluation.

See [`docs/architecture.md`](docs/architecture.md) for the initial boundaries.
