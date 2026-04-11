---
id: rules
title: Rules Reference
---

# Rules Reference

The greyproxy rule engine controls which network connections are allowed or denied. Rules are stored in SQLite, survive restarts, and take effect immediately without restarting the proxy.

## Rule Fields

| Field | Type | Description |
|-------|------|-------------|
| `container_pattern` | string | Glob to match against the source container or process name. `*` matches any container. |
| `destination_pattern` | string | Hostname or glob pattern to match (see [Pattern Syntax](#pattern-syntax)). |
| `port_pattern` | string | Port to match. `*` or an empty value means any port. |
| `action` | string | `"allow"` or `"deny"`. |
| `notes` | string | Optional free-form annotation. |
| `expires_at` | string | Optional RFC 3339 timestamp; the rule becomes inactive after this time. |

## Evaluation Order

For every incoming connection, the rule engine evaluates rules in this order:

1. **Deny rules first**: if the connection matches any deny rule, it is blocked immediately.
2. **Allow rules second**: if the connection matches any allow rule, it is forwarded.
3. **No match**: the connection is either blocked or queued as a **pending request** (see [Pending Requests](#pending-requests)).

Deny rules always take precedence over allow rules, regardless of creation order.

## Pattern Syntax

The `destination` field supports glob-style wildcard patterns:

| Pattern | Matches | Does not match |
|---------|---------|----------------|
| `registry.npmjs.org` | Exact hostname only | `foo.npmjs.org` |
| `*.npmjs.org` | Any subdomain of `npmjs.org` | `npmjs.org` itself |
| `*.*.npmjs.org` | Two levels of subdomains | `foo.npmjs.org` |
| `api.*` | Any hostname starting with `api.` | (no counter-example) |
| `*` | Every hostname | (no counter-example) |

Matching is case-insensitive.

:::tip
To allow both a domain and all its subdomains, create two rules: one for `example.com` and one for `*.example.com`.
:::

## Managing Rules

### From the Dashboard

Open [http://localhost:43080](http://localhost:43080), navigate to **Rules**, and use the form to add, edit, or delete rules. Changes take effect immediately.

### From the REST API

```bash
# List all rules
curl http://localhost:43080/api/rules

# Add an allow rule (any container, any port)
curl -X POST http://localhost:43080/api/rules \
  -H "Content-Type: application/json" \
  -d '{
    "container_pattern": "*",
    "destination_pattern": "*.npmjs.org",
    "port_pattern": "*",
    "action": "allow"
  }'

# Add a deny rule for a specific port and container
curl -X POST http://localhost:43080/api/rules \
  -H "Content-Type: application/json" \
  -d '{
    "container_pattern": "opencode",
    "destination_pattern": "telemetry.example.com",
    "port_pattern": "443",
    "action": "deny"
  }'

# Delete a rule by ID
curl -X DELETE http://localhost:43080/api/rules/42
```

See [REST API](./api) for full endpoint documentation.

## Pending Requests

When a connection doesn't match any rule, it enters a **pending** state rather than being silently dropped. This lets you review and approve (or deny) traffic interactively from the dashboard.

### Viewing Pending Requests

Open the **Pending Requests** tab in the dashboard. Each pending entry shows:

- The destination hostname and port
- The source (process or container, where available)
- Timestamp

Click **Allow** to add an allow rule for that destination, or **Deny** to block it permanently.

### Via the API

```bash
# List pending requests
curl http://localhost:43080/api/pending

# Approve a pending request (adds allow rule)
curl -X POST http://localhost:43080/api/pending/7/allow

# Deny a pending request (adds deny rule)
curl -X POST http://localhost:43080/api/pending/7/deny
```

## Building a Policy from Scratch

The recommended workflow for a new project or tool:

1. **Start with no allow rules**: everything is either blocked or pending
2. **Run your command** through greywall (or configure `ALL_PROXY` to point at greyproxy)
3. **Watch the Pending tab** in the dashboard
4. **Allow** the destinations your workflow legitimately needs
5. **Deny** anything suspicious or unexpected
6. **Export your rules** by querying `GET /api/rules` and committing the result

This iterative approach produces a minimal, auditable policy rather than a permissive one built by guessing.

## Persisting Rules

All rules are stored in the SQLite database under greyproxy's data directory (`~/.local/share/greyproxy/greyproxy.db` on Linux, `~/Library/Application Support/greyproxy/greyproxy.db` on macOS). They persist across restarts automatically, with no export step needed for ongoing use.

To share rules across machines or commit them to a repo, export them via the API:

```bash
curl http://localhost:43080/api/rules > greyproxy-rules.json
```

Importing rules back can be scripted with the `POST /api/rules` endpoint.

## Common Rule Patterns

### npm / Node.js

```
*.npmjs.org          allow  (npm registry)
*.github.com         allow  (GitHub tarballs)
registry.yarnpkg.com allow  (Yarn)
```

### Python / pip

```
pypi.org             allow
files.pythonhosted.org allow
*.pypi.org           allow
```

### Go modules

```
proxy.golang.org     allow
sum.golang.org       allow
*.pkg.go.dev         allow
```

### Block telemetry (deny before allow-all)

```
telemetry.example.com  deny  (blocks specific telemetry)
*                      allow (allow everything else)
```

Because deny rules are evaluated first, this combination blocks the telemetry endpoint while allowing all other traffic.
