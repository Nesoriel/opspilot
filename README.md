# OpsPilot

OpsPilot is a code-first, safety-oriented operations agent implemented in Go. Its core runtime stays provider-neutral while adapters integrate with the Volcengine AI ecosystem.

> Status: early engineering bootstrap. The current binary exposes a small, machine-readable, read-only tool runtime while the Ark model adapter and MCP layer are built next.

## Design goals

- Go-first runtime suitable for containers, Kubernetes, OpenClaw, Hermes, and other agents.
- Volcengine Ark, VikingDB, AgentKit, and VKE integrations behind replaceable interfaces.
- Read-only diagnostics by default; state-changing operations require explicit policy and approval.
- Structured JSON inputs and outputs for reliable agent-to-tool communication.
- Timeouts, step limits, tests, and observable execution rather than prompt-only safety.

## Current capabilities

- Provider-neutral agent loop with bounded steps and structured tool errors.
- Strongly defined tool registry with duplicate and schema validation.
- Read-only `dns_lookup` and SSRF-aware `http_probe` tools.
- Machine-readable CLI intended for agents and automation.

## Build and test

```bash
go test ./...
go build ./cmd/opspilot
```

## Use the tool runtime

```bash
go run ./cmd/opspilot tool list
go run ./cmd/opspilot tool run dns_lookup '{"host":"example.com"}'
go run ./cmd/opspilot tool run http_probe '{"url":"https://example.com"}'
```

Private, loopback, and link-local HTTP targets are blocked by default. Set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` only in a trusted environment where internal service diagnostics are intended.

## Roadmap

1. Ark Responses API adapter using CloudWeGo Eino `agenticark`.
2. Streaming event output and OpenTelemetry traces.
3. MCP client/server support and agent skill packaging.
4. Docker, Kubernetes, Prometheus, Loki, and TLS diagnostics.
5. PostgreSQL task state, VikingDB retrieval, approval gates, and VKE deployment.

See [`docs/architecture.md`](docs/architecture.md) for the initial boundaries.
