# OpsPilot

OpsPilot is a code-first, safety-oriented operations agent implemented in Go. Its core runtime stays provider-neutral while adapters integrate with the Volcengine AI ecosystem.

> Status: early development. The project now includes a bounded Agent Runtime, an Ark Responses API adapter, and a small set of machine-readable, read-only diagnostic tools.

## Design goals

- Go-first runtime suitable for containers, Kubernetes, OpenClaw, Hermes, and other agents.
- Volcengine Ark, VikingDB, AgentKit, and VKE integrations behind replaceable interfaces.
- Read-only diagnostics by default; state-changing operations require explicit policy and approval.
- Structured JSON inputs and outputs for reliable agent-to-tool communication.
- Timeouts, step limits, tests, and observable execution rather than prompt-only safety.

## Current capabilities

- Provider-neutral agent loop with bounded steps and structured tool errors.
- Volcengine Ark Responses API adapter through CloudWeGo Eino `agenticark`.
- Strongly defined tool registry with duplicate and schema validation.
- Read-only `dns_lookup` and SSRF-aware `http_probe` tools.
- Machine-readable CLI intended for agents and automation.
- Configuration validation and provider-error secret redaction.

## Build and test

```bash
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
go run ./cmd/opspilot agent run 'Resolve example.com and check whether its website is reachable.'
```

The command emits structured JSON containing the final answer, full message history, and executed step count. The Ark model can select from the registered read-only tools.

## Use the tool runtime directly

```bash
go run ./cmd/opspilot tool list
go run ./cmd/opspilot tool run dns_lookup '{"host":"example.com"}'
go run ./cmd/opspilot tool run http_probe '{"url":"https://example.com"}'
```

Private, loopback, and link-local HTTP targets are blocked by default. Set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` only in a trusted environment where internal service diagnostics are intended.

## Roadmap

1. Streaming Agent events and OpenTelemetry traces.
2. MCP client/server support and richer Agent skill packaging.
3. Docker, Kubernetes, Prometheus, Loki, and TLS diagnostics.
4. PostgreSQL task state and VikingDB retrieval.
5. Approval gates, AgentKit/VKE deployment, and production evaluation.

See [`docs/architecture.md`](docs/architecture.md) for the initial boundaries.
