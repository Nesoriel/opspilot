---
name: opspilot
description: Run safe, read-only infrastructure diagnostics or delegate a bounded diagnostic task to the OpsPilot Go agent.
---

# OpsPilot skill

Use OpsPilot when evidence is required from DNS, HTTP, TLS, a trusted local Docker Engine, a Kubernetes cluster configured with least-privilege credentials, or a trusted Prometheus endpoint. Treat its JSON output as evidence and preserve uncertainty.

## Preferred integration: MCP

Configure the OpsPilot binary as a stdio MCP server:

```json
{
  "mcpServers": {
    "opspilot": {
      "command": "/absolute/path/to/opspilot",
      "args": ["mcp", "stdio"],
      "env": {
        "OPSPILOT_DOCKER_SOCKET": "/var/run/docker.sock",
        "OPSPILOT_KUBECONFIG": "/absolute/path/to/kubeconfig",
        "OPSPILOT_KUBERNETES_CONTEXT": "production-readonly",
        "OPSPILOT_PROMETHEUS_URL": "https://prometheus.example.com",
        "OPSPILOT_PROMETHEUS_BEARER_TOKEN_FILE": "/absolute/path/to/prometheus-token"
      }
    }
  }
}
```

Discover and invoke the published tools through the MCP client. The tools are read-only and idempotent, but network, Docker, Kubernetes, and Prometheus tools still interact with privileged external systems. Do not enable private-network, Docker socket, Kubernetes credential, or Prometheus credential access unless the runtime is trusted.

Use `tls_inspect` when certificate expiry, trust, hostname coverage, TLS versions, cipher suites, or handshake failures may explain an incident. A successful handshake does not imply certificate verification succeeded: always inspect `verified` and `verification_error`.

For Docker incidents:

1. Call `docker_engine_info` to verify Engine connectivity and host-level runtime capabilities.
2. Call `docker_container_list` to identify stopped, restarting, unhealthy, or unexpectedly exposed containers.
3. Call `docker_container_inspect` only for the relevant container.

The Docker inspect result is intentionally incomplete. `warning_count` and `error_present` indicate that additional human investigation may be needed, but raw warning/error text, environment values, commands, labels, health output, log paths, and host mount source paths are not returned.

For Kubernetes incidents:

1. Call `kubernetes_cluster_info` to verify API connectivity, server version, node readiness, scheduling state, capacity, and node conditions.
2. Call `kubernetes_pod_list` for the relevant namespace. Use `*` only when a cluster-wide view is necessary.
3. Prioritize Pods that are not Ready, have high restart counts, are Pending/Failed, or report abnormal status reasons.
4. Call `kubernetes_pod_inspect` only for the relevant Pod.
5. Interpret Event `type` and `reason` patterns such as `FailedScheduling`, `BackOff`, `Unhealthy`, or `FailedMount`. Event messages and Pod/container free-text messages are intentionally absent.

Do not infer that an omitted field is empty. Pod logs, environment values, commands, arguments, labels, annotations, Secret/ConfigMap references, volume sources, and free-text messages are not collected. When `events_truncated` is true, state that the event sample was bounded.

For Prometheus incidents:

1. Call `prometheus_server_info` to verify connectivity, version, reload status, retention, series count, corruption count, and runtime pressure.
2. Call `prometheus_target_list` to identify down or unhealthy scrape targets. `error_present` indicates an omitted target error message.
3. Call `prometheus_metric_snapshot` only for a known metric relevant to the hypothesis.
4. Prefer exact matchers such as `job`, `namespace`, `pod`, `container`, `node`, or `instance` to narrow the result.
5. Use a safe aggregation only when the raw series view is not required. State when `truncated` is true.

`prometheus_metric_snapshot` does not accept arbitrary PromQL. Do not attempt to pass functions, range vectors, regular expressions, subqueries, offsets, or arbitrary label names. Scrape URLs, discovered labels, arbitrary labels, raw target errors, warning/info text, runtime hostname, working directory, and credentials are intentionally absent.

## Delegate a diagnostic task

```bash
opspilot agent run 'Check Kubernetes node readiness, unhealthy Pods, and Prometheus scrape targets.'
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
opspilot tool run kubernetes_cluster_info '{"node_limit":100}'
opspilot tool run kubernetes_pod_list '{"namespace":"operations","limit":100}'
opspilot tool run kubernetes_pod_inspect '{"namespace":"operations","pod":"web-0","event_limit":50}'
opspilot tool run prometheus_server_info '{}'
opspilot tool run prometheus_target_list '{"limit":100}'
opspilot tool run prometheus_metric_snapshot '{"metric":"up","matchers":{"job":"node"},"aggregation":"sum","group_by":["instance"],"limit":100}'
```

Check `ok` before reading `data`. Do not set `OPSPILOT_HTTP_ALLOW_PRIVATE=true`, `OPSPILOT_TLS_ALLOW_PRIVATE=true`, or `OPSPILOT_PROMETHEUS_ALLOW_HTTP=true` outside a trusted internal environment. Access to a Docker socket is itself privileged even though OpsPilot sends only read-only requests. Kubernetes credentials must use least-privilege RBAC and must not grant Secrets, Pod logs, exec, attach, port-forward, or mutating verbs. Prometheus credentials must be read-only and scoped to the configured endpoint.

Do not fabricate tool results. When a command fails, preserve the returned error class and continue with other read-only evidence when possible. Never expose Ark credentials, kubeconfig contents, ServiceAccount tokens, Prometheus bearer tokens, or raw infrastructure credentials in prompts, logs, or tool arguments. In MCP mode, treat stdout as protocol-only and send diagnostics to stderr.
