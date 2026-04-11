---
id: quickstart
title: Quickstart
---

# Greyproxy Quickstart

## Installation

### Homebrew (macOS)

```bash
brew tap greyhavenhq/tap
brew install greyproxy
```

On macOS the Homebrew cask runs `greyproxy install -f` automatically after installation, so the launchd user agent is registered and the dashboard is reachable on port 43080 as soon as `brew install` finishes. You can skip the "Install as a Service" section below if you installed this way.

### Build from Source

```bash
git clone https://github.com/greyhavenhq/greyproxy.git
cd greyproxy
go build ./cmd/greyproxy
```

On macOS, codesign the freshly built binary before running `install` so that Gatekeeper does not quarantine it:

```bash
codesign --sign - --force ./greyproxy
```

### Via Greywall

If you're using [Greywall](../greywall), you can install and start greyproxy automatically:

```bash
greywall setup
```

## Install as a Service

Install the binary to `~/.local/bin/` and register it as a systemd user service (Linux) or a launchd user agent (macOS):

```bash
./greyproxy install
```

`install` copies the binary, registers the service, generates and trusts a CA certificate for HTTPS interception (asking for sudo if needed), and starts the service. The dashboard is then available at [http://localhost:43080](http://localhost:43080).

If greyproxy was installed via Homebrew or a symlink manager such as mise, `install` detects that and registers the existing binary in place rather than copying it, so later upgrades keep the service in sync.

To skip the interactive prompts during install or uninstall, pass `-f`:

```bash
greyproxy install -f
greyproxy uninstall -f
```

### Trusting the CA certificate

HTTPS inspection, LLM conversation tracking, header redaction, and credential substitution all rely on greyproxy terminating TLS with its own CA. `greyproxy install` handles this automatically, but you can also manage the certificate directly:

```bash
greyproxy cert generate      # create a new CA keypair
greyproxy cert install       # add it to the OS trust store (sudo)
greyproxy cert uninstall     # remove it from the OS trust store
greyproxy cert reload        # reload the live cert with no restart
```

To remove everything:

```bash
greyproxy uninstall
```

## Run in Foreground

To run the server directly without installing as a service:

```bash
greyproxy serve
```

Or with a custom configuration file:

```bash
greyproxy serve -C greyproxy.yml
```

## Service Management

Once installed, manage the service with:

```bash
greyproxy service status
greyproxy service start
greyproxy service stop
greyproxy service restart
```

## Access the Dashboard

Once running, open [http://localhost:43080](http://localhost:43080) in your browser to access the dashboard.

The dashboard shows:

- Real-time traffic overview
- Pending requests awaiting approval or denial
- Rule management
- Activity and transaction logs
- LLM conversations captured from intercepted traffic
- Settings (notifications, MITM, credentials, header redaction, endpoint rules)

## Default Ports

| Service       | Port    |
|---------------|---------|
| Dashboard/API | `43080` |
| HTTP Proxy    | `43051` |
| SOCKS5 Proxy  | `43052` |
| DNS Proxy     | `43053` |

## Next Steps

- [Configuration](./configuration): customize ports, database location, and runtime settings.
- [Dashboard](./dashboard): learn about the web UI.
- [LLM Conversations](./conversations): inspect traffic from Claude Code, Codex, Aider, OpenCode, and other AI tools.
- [Credential Substitution](./credentials): keep API keys out of sandboxed processes.
- [REST API](./api): automate greyproxy.
- [Using with Greywall](./using-with-greywall): integrate with the greywall sandbox.
