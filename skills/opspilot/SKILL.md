---
name: opspilot
description: Run safe, read-only infrastructure diagnostics or delegate a bounded diagnostic task to the OpsPilot Go agent.
---

# OpsPilot skill

Use OpsPilot when evidence is required from DNS, HTTP, TLS, or a trusted local Docker Engine. Treat its JSON output as evidence and preserve uncertainty.

## Preferred integration: MCP

Configure the OpsPilot binary as a stdio MCP server:

```json
{
  "mcpServers": {
    "opspilot": {
      "command": "/absolute/path/to/opspilot",
      "args": ["mcp", "stdio"],
      "env": {
        "OPSPILOT_DOCKER_SOCKET": "/var/run/docker.sock"
      }
    }
  }
}
```

Discover and invoke the published tools through the MCP client. The tools are read-only and idempotent, but network and Docker tools still interact with privileged external systems. Do not enable private-network access or Docker socket access unless the runtime is trusted.

Use `tls_inspect` when certificate expiry, trust, hostname coverage, TLS versions, cipher suites, or handshake failures may explain an incident. A successful handshake does not imply certificate verification succeeded: always inspect `verified` and `verification_error`.

For Docker incidents:

1. Call `docker_engine_info` to verify Engine connectivity and host-level runtime capabilities.
2. Call `docker_container_list` to identify stopped, restarting, unhealthy, or unexpectedly exposed containers.
3. Call `docker_container_inspect` only for the relevant container.

The Docker inspect result is intentionally incomplete. `warning_count` and `error_present` indicate that additional human investigation may be needed, but raw warning/error text, environment values, commands, labels, health output, log paths, and host mount source paths are not returned.

## Delegate a diagnostic task

```bash
opspilot agent run 'Check the local Docker Engine and identify unhealthy or stopped containers.'
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
opspilot tool run docker_engine_info '{}'
opspilot tool run docker_container_list '{"all":true,"limit":100}'
opspilot tool run docker_container_inspect '{"container":"web"}'
```

Check `ok` before reading `data`. Do not set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` or `OPSPILOT_TLS_ALLOW_PRIVATE=true` outside a trusted internal environment. Access to a Docker socket is itself privileged even though OpsPilot sends only read-only requests; never expose the socket to an untrusted Agent process or container.

Do not fabricate tool results. When a command fails, preserve the returned error and continue with other read-only evidence when possible. Never expose Ark credentials in prompts, logs, or tool arguments. In MCP mode, treat stdout as protocol-only and send diagnostics to stderr.
