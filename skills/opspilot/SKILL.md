---
name: opspilot
description: Run safe, read-only infrastructure diagnostics or delegate a bounded diagnostic task to the OpsPilot Go agent.
---

# OpsPilot skill

Use OpsPilot when evidence is required from DNS, HTTP, or TLS endpoints. Treat its JSON output as evidence and preserve uncertainty.

## Preferred integration: MCP

Configure the OpsPilot binary as a stdio MCP server:

```json
{
  "mcpServers": {
    "opspilot": {
      "command": "/absolute/path/to/opspilot",
      "args": ["mcp", "stdio"]
    }
  }
}
```

Discover and invoke the published tools through the MCP client. The tools are read-only and idempotent, but network tools still interact with external systems. Do not enable private-network access unless the runtime is trusted.

Use `tls_inspect` when certificate expiry, trust, hostname coverage, TLS versions, cipher suites, or handshake failures may explain an incident. A successful handshake does not imply certificate verification succeeded: always inspect `verified` and `verification_error`.

## Delegate a diagnostic task

```bash
opspilot agent run 'Resolve example.com, inspect its TLS certificate, and check whether its website is reachable.'
```

This requires `ARK_MODEL_ID` and Ark credentials in the process environment. The command writes JSON containing the final answer, message history, and step count to stdout.

For live progress without mixing events into the final result:

```bash
opspilot agent run --events=jsonl 'Inspect example.com.' 2>events.jsonl
```

Consume stderr as a JSONL event stream and stdout as the final result. Events contain lifecycle metadata only; they do not contain prompts, tool arguments, tool results, or credentials.

## Discover tools without MCP

```bash
opspilot tool list
```

## Run a tool directly

```bash
opspilot tool run dns_lookup '{"host":"example.com"}'
opspilot tool run http_probe '{"url":"https://example.com"}'
opspilot tool run tls_inspect '{"host":"example.com","port":443}'
```

Check `ok` before reading `data`. Do not set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` or `OPSPILOT_TLS_ALLOW_PRIVATE=true` outside a trusted internal environment.

Do not fabricate tool results. When a command fails, preserve the returned error and continue with other read-only evidence when possible. Never expose Ark credentials in prompts, logs, or tool arguments. In MCP mode, treat stdout as protocol-only and send diagnostics to stderr.
