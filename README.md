# OpsPilot

OpsPilot is a code-first, safety-oriented operations agent implemented in Go. Its core runtime stays provider-neutral while adapters integrate with the Volcengine AI ecosystem.

> Status: early development. The project includes a bounded Agent Runtime, an Ark Responses API adapter, an MCP stdio server, privacy-safe runtime events, optional OpenTelemetry tracing, and a small set of machine-readable, read-only diagnostic tools.

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
- Read-only `dns_lookup` and SSRF-aware `http_probe` tools.
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
go run ./cmd/opspilot agent run 'Resolve example.com and check whether its website is reachable.'
```

The command writes the final structured result to stdout. The Ark model can select from the registered read-only tools.

### Stream lifecycle events

```bash
go run ./cmd/opspilot agent run --events=jsonl \
  'Resolve example.com and check whether its website is reachable.' \
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
        "OPSPILOT_HTTP_ALLOW_PRIVATE": "false"
      }
    }
  }
}
```

The server publishes each Registry tool with its existing JSON Schema and explicit read-only/idempotent annotations. Tool results are returned as text and, when the result is a JSON object, as MCP structured content. stdout is reserved exclusively for MCP protocol frames; warnings and failures go to stderr.

## Use the tool runtime directly

```bash
go run ./cmd/opspilot tool list
go run ./cmd/opspilot tool run dns_lookup '{"host":"example.com"}'
go run ./cmd/opspilot tool run http_probe '{"url":"https://example.com"}'
```

Private, loopback, and link-local HTTP targets are blocked by default. Set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` only in a trusted environment where internal service diagnostics are intended.

## Roadmap

1. MCP client support and richer Agent skill packaging.
2. Docker, Kubernetes, Prometheus, Loki, and TLS diagnostics.
3. PostgreSQL task state and VikingDB retrieval.
4. Approval gates, AgentKit/VKE deployment, and production evaluation.

See [`docs/architecture.md`](docs/architecture.md) for the initial boundaries.
