# Architecture

## Core principle

OpsPilot owns its execution policy and state transitions. Volcengine services accelerate model inference, retrieval, deployment, and observability, but do not define the domain model.

```text
OpenClaw / Hermes / MCP client / API / CLI
                  |
                  v
          OpsPilot process boundary
          - Agent CLI
          - Tool CLI
          - MCP stdio server
                  |
                  v
          OpsPilot Go runtime
          - bounded agent loop
          - policy and approval
          - shared tool registry
          - structured events
                  |
        +---------+----------+
        |         |          |
        v         v          v
      Models    Tools    Observability
      Ark/Eino  network  JSONL events
      others    Docker   OpenTelemetry
                K8s
                Prometheus
        |         |          |
        v         v          v
      Volcengine local and  OTLP collector
      Ark        cloud APIs cloud backend
```

## Package boundaries

- `internal/agent`: provider-neutral messages, model/tool contracts, registry, bounded runtime, and lifecycle event definitions.
- `internal/models`: provider adapters such as the Ark Responses API adapter. Provider SDKs must not enter `internal/agent`.
- `internal/dockerapi`: a bounded, read-only Docker Engine API adapter over a trusted local Unix socket. It owns API negotiation, transport errors, response limits, and redacted response projections.
- `internal/kubeapi`: a lazy, bounded, read-only Kubernetes adapter built on official client-go. It owns safe configuration loading, API error classification, fixed resource queries, and redacted response projections.
- `internal/promapi`: a lazy, bounded, read-only Prometheus `/api/v1` adapter. It owns endpoint validation, fixed requests, safe query generation, transport limits, and privacy-aware projections.
- `internal/tools`: read-only operational tools. Tools must validate JSON strictly and respect `context.Context`.
- `internal/mcpserver`: adapts the shared Registry to the official MCP Go SDK without duplicating tool implementations.
- `internal/observability`: observer composition, privacy-safe JSONL records, and OpenTelemetry span translation.
- `cmd/opspilot`: machine-readable process boundary. Human-friendly UI is not a current priority.
- `skills`: instructions and schemas for external agents that invoke OpsPilot.

## MCP boundary

The MCP server is a transport adapter, not a second execution engine.

- Tool names, descriptions, and JSON Schemas come from `agent.Registry`.
- MCP calls execute the same `agent.Tool` implementation used by the CLI and Agent Runtime.
- Published annotations mark current tools as read-only and idempotent; network, Docker, Kubernetes, and Prometheus tools are conservatively marked open-world because they interact with systems outside the process.
- Each MCP tool call is bounded by context cancellation and a server-side timeout.
- JSON object results are returned as both text content and MCP structured content.
- Tool failures use `CallToolResult.IsError`; unknown tool names remain protocol-level errors.
- stdio stdout is reserved exclusively for MCP protocol frames. Logs and startup failures go to stderr.
- The first transport is local stdio. Remote HTTP, authentication, and multi-tenant policy remain separate work.

## Docker boundary

Docker support is split between a transport/API adapter and Agent-facing tools.

- The first implementation accepts only an absolute local Unix socket path. TCP, HTTP, HTTPS, SSH, named-pipe, and relative targets are rejected.
- The client requests `/version` without a version prefix, validates the reported API version, and prefixes later read-only calls with that version.
- Only GET operations for Engine information, container listing, and container inspection are implemented.
- Docker CLI execution is not used, and the Agent cannot provide an arbitrary API path or HTTP method.
- Response bodies are bounded before JSON decoding. Container lists are bounded both in the API query and again in the tool layer.
- The API uses open response schemas; the adapter declares only fields required by the diagnostic projection and ignores unknown fields.
- Raw inspect data is never returned. Environment values, commands, arguments, labels, health-check output, log paths, mount source paths, and free-text runtime errors are excluded.
- Free-text daemon warnings are represented only by `warning_count`. Free-text container errors are represented only by `error_present`.
- A Docker socket remains a privileged host capability. Read-only application code is not an operating-system-enforced read-only socket permission. Deployment policy must restrict which process receives the socket.

Container logs are intentionally outside this first boundary because they frequently contain credentials, request data, database records, and other application output. A future log tool requires explicit line/byte limits, timestamp controls, and a separate redaction policy.

## Kubernetes boundary

Kubernetes support uses the official client-go adapter behind fixed Agent tools.

- Client initialization is lazy. A missing or invalid cluster configuration does not prevent non-Kubernetes tools, CLI discovery, or the MCP server from starting.
- Outside a cluster, an explicit absolute kubeconfig path or the standard kubeconfig loading rules may be used. Inside a cluster, the mounted ServiceAccount token and CA are used.
- Raw kubeconfig is validated before client-go constructs transports. HTTP API servers, insecure TLS verification, proxy URLs, exec credential plugins, legacy auth-provider plugins, and user impersonation are rejected.
- Proxy use is explicitly disabled in the resulting REST configuration, including ambient HTTP proxy environment variables.
- The client applies request timeouts, QPS, burst, list limits, and a fixed user agent.
- The Agent cannot provide an API server URL, kubeconfig path, context, resource type, selector, arbitrary API path, or HTTP method in tool arguments.
- The first fixed queries are server version, Nodes, Pods, one Pod, and Events associated with that Pod UID.
- Pod and node lists are limited at the API layer and again in the tool layer.
- Raw Kubernetes objects are never returned. Labels, annotations, environment values, commands, arguments, volume source details, Secret and ConfigMap references, logs, and free-text status/condition/container/Event messages are excluded.
- Event output is aggregated by type and reason. Machine-classified reasons and exit codes are retained; free-text messages are not.
- Generic API error classes are returned without raw server messages, bearer tokens, URLs, object paths, or admission details.
- A minimal ClusterRole grants only GET on `/version`, GET/LIST on Nodes and Pods, and LIST on Events. It excludes Secrets and `pods/log`.

RBAC is the cluster-enforced boundary. Code-level GET-only behavior and projection redaction complement RBAC but do not replace it.

## Prometheus boundary

Prometheus support uses a small standard-library HTTP adapter behind three fixed Agent tools.

- Initialization is lazy. Missing Prometheus configuration does not block CLI discovery, MCP startup, or unrelated tools.
- The base URL is process configuration, never a tool argument. HTTPS is required unless a trusted deployment explicitly enables HTTP.
- URL user information, query strings, fragments, redirects, ambient proxies, and insecure TLS are not allowed. HTTPS requires TLS 1.2 or newer.
- Optional bearer authentication reads a bounded token from an absolute file for each request, supporting rotation without exposing the value.
- The only endpoints are build information, runtime information, active targets, and instant query. Configuration, flags, rules, alerts, labels, series enumeration, and administration endpoints are absent.
- Status and target requests use GET. Metric matchers are submitted using the Prometheus URL-encoded POST form so they do not appear in the request URL.
- Arbitrary PromQL is not a tool interface. The adapter generates a selector from one validated metric name, up to eight exact label matchers from a fixed diagnostic allowlist, an optional safe aggregation, up to five grouping labels, and a hard series limit.
- Request timeout, Prometheus query timeout, response bytes, target count, and series count are all bounded. Result limits are applied again locally.
- Raw API envelopes and objects are never returned. Scrape URLs, discovered labels, arbitrary labels, target error text, runtime hostname and working directory, warning/info text, and raw API errors are excluded.
- Target errors are represented by `error_present`. API warnings and infos are represented by counts only.
- Sample timestamps must be finite and within the RFC3339 year range. Unexpected result types, malformed values, oversized responses, and invalid timestamps are rejected.

The Prometheus endpoint and bearer token remain privileged operational credentials. The process must receive only the minimum read-only access required by its deployment.

## Observability boundary

The Agent Runtime emits provider-neutral lifecycle events with a run ID, timestamp, duration, step, tool name, call ID, and error value. Observers translate these events for different consumers.

- JSONL events go to stderr while the final Agent result remains on stdout.
- Trace spans represent the Agent run, model generations, and tool executions.
- Default telemetry excludes prompts, model text, tool arguments, tool results, credentials, and raw error messages.
- Observer panics, exporter initialization failures, and exporter shutdown failures must not fail the diagnostic run.
- OTLP export is opt-in and reads standard OpenTelemetry environment variables.

## Safety model

1. Tools are read-only unless explicitly classified otherwise.
2. Arbitrary shell strings and arbitrary infrastructure API paths are not tool interfaces.
3. Every call has a timeout and a bounded Agent run has a maximum step count.
4. Network tools deny private, loopback, link-local, multicast, and unspecified targets unless a trusted deployment explicitly enables them.
5. Docker tools require a trusted local Unix socket and expose only a fixed allowlist of GET operations and output fields.
6. Kubernetes tools require validated trusted credentials, fixed resource queries, redacted projections, and least-privilege RBAC.
7. Prometheus tools require a trusted configured endpoint, fixed API calls, generated bounded queries, redacted projections, and bounded output.
8. Tool failures are returned to the model or MCP client as structured data; they do not silently disappear.
9. Future mutating tools must pass policy evaluation and an approval checkpoint before execution.
10. Observability metadata must not become a covert channel for prompts, credentials, or complete tool data.
11. Protocol transports must keep framing channels free from unrelated logs or diagnostics.

## Volcengine integration plan

- Ark Responses API: model inference, function calling, streaming, and context caching.
- Eino: typed Go composition and Ark adapter; adopted behind OpsPilot interfaces.
- VikingDB: operational runbooks, incident cases, and evidence retrieval.
- AgentKit/VKE: production runtime and deployment after the local runtime is testable.
- Volcengine logging and monitoring: receive OTLP-compatible telemetry or consume structured events without changing the Agent Runtime.
