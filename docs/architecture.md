---
id: architecture
title: How It Works
---

# How Greyproxy Works

Greyproxy is a single binary that bundles a multi-protocol proxy server, a SQLite-backed rule engine, a REST API, and a web dashboard. All components start from a single `greyproxy serve` command.

## Request Lifecycle

Every network request passing through greyproxy goes through the same pipeline:

```
Client Request
      │
      ▼
 ┌──────────────────────────────────┐
 │  Protocol handler                │
 │  (HTTP, SOCKS5, or DNS)          │
 └──────────────┬───────────────────┘
                │
                ▼
 ┌──────────────────────────────────┐
 │  DNS resolution + enrichment     │
 │  Resolve hostname → IP           │
 │  Attach hostname to connection   │
 └──────────────┬───────────────────┘
                │
                ▼
 ┌──────────────────────────────────┐
 │  Rule engine                     │
 │  1. Check deny rules             │
 │  2. Check allow rules            │
 │  3. No match → PENDING / BLOCK   │
 └──────────┬────────────┬──────────┘
            │            │
         ALLOW          DENY / PENDING
            │            │
            ▼            ▼
     Forward to      Block or queue
     destination     for review
```

For HTTPS traffic, a fourth stage sits between the protocol handler and the rule engine: a MITM interceptor that terminates TLS with greyproxy's own CA, runs dissectors over the decrypted request and response, and (when applicable) substitutes credentials before handing the connection off to the rule engine.

## Components

### Protocol Handlers

Greyproxy exposes three proxy protocols simultaneously, all manageable through the same dashboard and rule engine:

| Protocol | Port | Use case |
|----------|------|----------|
| HTTP/HTTPS proxy | `43051` | Browser-style proxy, `HTTP_PROXY` env var |
| SOCKS5 proxy | `43052` | General TCP proxying, used by greywall |
| DNS proxy | `43053` | DNS query capture, caching, hostname enrichment |

The HTTP and SOCKS5 handlers are built on [GOST v3](https://github.com/go-gost/gost), a feature-rich tunnel and proxy toolkit. Greyproxy adds the management layer (rule engine, dashboard, API, database) on top.

### Rule Engine

The rule engine evaluates every outgoing connection against an ordered set of rules stored in SQLite. Evaluation is fast and in-memory; the SQLite store is only hit on changes.

Rules match on:
- **Destination hostname** (exact or glob pattern, e.g., `*.npmjs.org`)
- **Port** (specific port or any)
- **Action**: `allow` or `deny`

Evaluation order:
1. Deny rules are checked first
2. Allow rules are checked second
3. If no rule matches, the connection is either blocked or placed in **pending** state (depending on configuration)

### HTTPS Inspection (MITM)

Greyproxy includes a built-in certificate authority that it uses to terminate TLS connections, inspect the plaintext HTTP exchange, and then forward the request upstream. The CA certificate is generated during `greyproxy install` (or on demand with `greyproxy cert generate`) and trusted in the OS trust store, so browsers and CLI tools see valid certificates.

MITM can be toggled at runtime via the settings API (`mitmEnabled`). It is what powers header redaction, transaction capture, LLM conversation tracking, and credential substitution. When MITM is disabled, greyproxy still forwards HTTPS traffic transparently, but it cannot look inside it.

The live CA can be reloaded with `greyproxy cert reload` without restarting the service.

### Conversation Assembler

When the MITM layer decodes a request, greyproxy checks it against an endpoint registry to decide whether a dissector should parse it. Dissectors handle specific LLM API shapes (Anthropic Messages, OpenAI Responses, OpenAI Chat Completions, Google Gemini, and WebSocket variants). Their output is fed into client adapters that detect the coding tool behind the traffic and group transactions into sessions.

The result is a Conversations view in the dashboard that reconstructs full LLM sessions from raw HTTP traffic. See [LLM Conversations](./conversations) for the user-facing guide.

### Credential Substitution

Greyproxy can hold a set of placeholder-to-real-value mappings in memory and replace every occurrence of a placeholder with its real value in outgoing HTTP headers and query parameters. Substitution runs at the MITM layer, after the transaction headers have been cloned for storage, so the dashboard only ever sees the placeholder.

Session credentials are registered by greywall for the lifetime of a sandboxed process. Global credentials are persisted and encrypted on disk with a per-installation key. See [Credential Substitution](./credentials).

### Docker Name Resolution (optional)

When the optional Docker integration is enabled, greyproxy queries the Docker or Podman socket to resolve source IPs of inbound connections to real container names. Rules can then match by container name instead of an opaque IP, and the activity log shows `docker-backend-1` rather than `unknown-172.17.0.2`. The resolution cache has a configurable TTL.

### DNS Caching and Enrichment

The DNS proxy serves two purposes:

1. **Caching**: resolves and caches DNS responses to reduce latency for repeated lookups. The cache is persisted to SQLite so it survives restarts.
2. **Hostname enrichment**: associates resolved IP addresses with their hostnames. When a SOCKS5 connection arrives with only an IP address, greyproxy can look up the original hostname and apply domain-based rules correctly.

The upstream resolver is auto-detected from the host (`/etc/resolv.conf` on Linux and macOS, the registry on Windows), with a fallback to `1.1.1.1:53`. This is why greywall routes DNS through greyproxy (`localhost:43053`) in addition to TCP traffic.

### Pending Requests

When a connection doesn't match any allow or deny rule, it can be placed in a **pending** queue rather than silently dropped. This enables an interactive workflow:

1. Run your command with greywall (network blocked by default)
2. Watch the greyproxy dashboard, where blocked destinations appear in Pending
3. Click Allow or Deny per destination
4. Approved destinations are added as allow rules and persist across restarts

This is the recommended way to build up an allow list for a new project or tool.

### Dashboard and API

The dashboard and REST API are served from the same port as the management interface (`43080`). The dashboard is a single-page app with all assets (HTML, CSS, JavaScript, fonts, and icons) embedded in the greyproxy binary. There is no CDN dependency or separate frontend server.

The API and dashboard communicate via:
- **REST endpoints** for rule CRUD and log queries
- **WebSocket** for real-time push updates (new pending requests, live log tail)

### Storage

All state is stored in a single SQLite database file under greyproxy's data directory (`~/.local/share/greyproxy/greyproxy.db` on Linux, `~/Library/Application Support/greyproxy/greyproxy.db` on macOS). The database holds:

- Rules (allow and deny)
- Pending requests
- Connection logs
- HTTP and WebSocket transactions
- LLM conversations and endpoint rules
- DNS cache entries

The MITM CA (`ca-cert.pem`, `ca-key.pem`), runtime settings (`settings.json`), and the encryption key used for global credentials (`session.key`) live alongside the database in the same directory. SQLite makes the database trivially portable and easy to inspect with standard tools.

## Integration with Greywall

When greywall is configured to use greyproxy, the traffic flow looks like this:

```
Sandboxed command
       │ (all TCP/UDP)
       ▼
  tun2socks (Linux) / env vars (macOS)
       │
       ▼
  Greyproxy SOCKS5 :43052
       │
       ├─ DNS queries → Greyproxy DNS :43053
       │
       ▼
  Rule engine
       │
       ├─ ALLOW → forward to Internet
       └─ DENY / PENDING → block or queue for review
```

On Linux, greywall creates a TUN network device inside the sandbox and uses `tun2socks` to forward all traffic (including non-HTTP protocols like raw TCP, WebSocket, etc.) to greyproxy's SOCKS5 port. On macOS, proxy environment variables are used instead, which only captures traffic from applications that respect them.

## Relationship with GOST

Greyproxy is a fork of [GOST (GO Simple Tunnel)](https://github.com/go-gost/gost). The core TCP handling, SOCKS5 implementation, and protocol infrastructure all come from GOST v3. Greyproxy adds:

- The rule engine and SQLite backend
- The web dashboard with real-time WebSocket updates
- The pending request queue
- The REST API and control WebSocket
- HTTPS inspection with a managed CA
- The conversation assembler for LLM traffic
- Credential substitution
- Header redaction
- DNS enrichment and persistent caching
- Optional Docker/Podman container name resolution
- The `install`, `uninstall`, `cert`, and `service` CLI commands for managed deployment

For documentation on the underlying proxy and tunnel capabilities beyond what greyproxy exposes, refer to the [GOST documentation](https://gost.run/en/).
