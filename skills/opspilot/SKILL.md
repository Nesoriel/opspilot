---
name: opspilot
description: Run safe, read-only infrastructure diagnostics or delegate a bounded diagnostic task to the OpsPilot Go agent.
---

# OpsPilot skill

Use OpsPilot when evidence is required from DNS or HTTP endpoints. Treat its JSON output as evidence and preserve uncertainty.

## Delegate a diagnostic task

```bash
opspilot agent run 'Resolve example.com and check whether its website is reachable.'
```

This requires `ARK_MODEL_ID` and Ark credentials in the process environment. The command emits JSON containing the final answer, message history, and step count.

## Discover tools

```bash
opspilot tool list
```

## Run a tool directly

```bash
opspilot tool run dns_lookup '{"host":"example.com"}'
opspilot tool run http_probe '{"url":"https://example.com"}'
```

Check `ok` before reading `data`. Do not set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` outside a trusted internal environment.

Do not fabricate tool results. When a command fails, preserve the returned error and continue with other read-only evidence when possible. Never expose Ark credentials in prompts, logs, or tool arguments.
