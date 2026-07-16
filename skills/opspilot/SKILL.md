---
name: opspilot
description: Run safe, read-only infrastructure diagnostics through the OpsPilot Go binary.
---

# OpsPilot skill

Use OpsPilot when evidence is required from DNS or HTTP endpoints. Treat its JSON output as evidence, not as a final diagnosis.

## Discover tools

```bash
opspilot tool list
```

## Run a tool

```bash
opspilot tool run dns_lookup '{"host":"example.com"}'
opspilot tool run http_probe '{"url":"https://example.com"}'
```

The command emits JSON. Check `ok` before reading `data`. Do not set `OPSPILOT_HTTP_ALLOW_PRIVATE=true` outside a trusted internal environment.

Do not fabricate tool results. When a command fails, preserve the returned error and continue with other read-only evidence when possible.
