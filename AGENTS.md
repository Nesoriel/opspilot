# Agent development rules

## Scope

OpsPilot is a Go-first operations agent and agent-readable diagnostics runtime. Keep the core provider-neutral and integrate Volcengine through adapters.

## Required workflow

- Read `README.md` and `docs/architecture.md` before changing code.
- Verify current external SDK APIs against official repositories or documentation.
- Run `gofmt -w`, `go vet ./...`, and `go test ./...` for Go changes.
- Add tests for failure paths, timeouts, policy boundaries, and malformed model/tool data.
- Keep pull requests focused and explain any new dependency.

## Safety constraints

- Do not introduce arbitrary shell execution.
- Do not make mutating infrastructure operations available without a policy interface and explicit approval state.
- Do not weaken private-network or SSRF protections merely to make a test pass.
- Never log API keys, access keys, secrets, raw authorization headers, or complete sensitive tool output.
- Model output is untrusted input. Validate every tool name and argument in code.

## Architecture constraints

- `internal/agent` must not import a provider SDK.
- Provider adapters implement the interfaces owned by OpsPilot.
- Built-in tools must accept `context.Context`, use strict JSON decoding, and return structured JSON.
- Prefer the standard library. Add dependencies only when they materially reduce risk or implement an adopted integration such as Eino, MCP, OpenTelemetry, or an official cloud SDK.
