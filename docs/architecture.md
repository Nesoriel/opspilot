# Architecture

## Core principle

OpsPilot owns its execution policy and state transitions. Volcengine services accelerate model inference, retrieval, deployment, and observability, but do not define the domain model.

```text
OpenClaw / Hermes / API / CLI
             |
             v
      OpsPilot Go runtime
      - bounded agent loop
      - policy and approval
      - structured events
             |
      +------+------+
      |             |
      v             v
Model adapters   Tool registry
Ark/Eino         built-in Go tools
other models     MCP tools
      |             |
      v             v
Volcengine       Docker / Kubernetes
Ark              Prometheus / Loki
                 HTTP / DNS / TLS
```

## Package boundaries

- `internal/agent`: provider-neutral messages, model/tool contracts, registry, and bounded runtime.
- `internal/tools`: read-only operational tools. Tools must validate JSON strictly and respect `context.Context`.
- `cmd/opspilot`: machine-readable process boundary. Human-friendly UI is not a current priority.
- `skills`: instructions and schemas for external agents that invoke OpsPilot.

## Safety model

1. Tools are read-only unless explicitly classified otherwise.
2. Arbitrary shell strings are not a tool interface.
3. Every call has a timeout and a bounded agent run has a maximum step count.
4. Network tools deny private, loopback, link-local, multicast, and unspecified targets unless a trusted deployment explicitly enables them.
5. Tool failures are returned to the model as structured data; they do not silently disappear.
6. Future mutating tools must pass policy evaluation and an approval checkpoint before execution.

## Volcengine integration plan

- Ark Responses API: model inference, function calling, streaming, and context caching.
- Eino: typed Go composition and Ark adapter; adopted behind OpsPilot interfaces.
- VikingDB: operational runbooks, incident cases, and evidence retrieval.
- AgentKit/VKE: production runtime and deployment after the local runtime is testable.
- Volcengine logging and monitoring: production telemetry export through OpenTelemetry-compatible boundaries where possible.
