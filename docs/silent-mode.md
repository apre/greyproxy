---
id: silent-mode
title: Silent Mode
---

# Silent Mode

Silent mode is a short-lived override that either allows or denies **every** connection without consulting the rule engine or creating pending requests. It is modelled on Little Snitch's silent mode and is designed for two situations:

- **Allow all** while you run a one-off task that you do not want to interrupt with pending-request prompts (a large dependency install, an unfamiliar CLI tool, a debugging session).
- **Deny all** when you want to quickly cut outbound traffic for a specific window without editing rules or stopping the proxy.

Silent mode intentionally bypasses ACL evaluation entirely. Rules, pending requests, and the learning workflow are all short-circuited while it is active; only the current mode decides the outcome. Every connection is still written to the activity log, so you retain an audit trail of what happened during the window.

State is not persisted: restarting greyproxy always reverts to normal mode, and there is a hard ceiling of 8 hours on any single activation. Both choices are deliberate, silent mode is meant to be temporary.

## Behaviour at a glance

| Setting           | Effect                                                                 |
|-------------------|------------------------------------------------------------------------|
| Normal (off)      | Rules and pending-request flow apply as usual.                         |
| Silent **allow**  | All connections permitted; no pending requests created; logged.        |
| Silent **deny**   | All connections blocked; no pending requests created; logged.          |
| Restart           | Silent mode is cleared; the proxy comes back up in normal mode.        |
| Maximum duration  | 8 hours per activation (or "until next restart").                      |

## Activating from the dashboard

The shield icon in the top navigation opens the silent mode menu. It offers two groups:

- **Allow All Connections**: 5 min, 15 min, 30 min, 1 hour, 2 hours, or until next restart.
- **Deny All Connections**: 5 min, 15 min, 30 min, 1 hour, or until next restart.

While active, a banner at the top of the dashboard shows the current mode and a countdown to expiry, and the menu replaces the duration options with a **Resume Normal Mode** button. Clicking it clears silent mode immediately.

## Activating from the CLI

Silent mode can also be set at startup via a flag on `greyproxy serve`:

```bash
greyproxy serve -C config.yaml -silent-allow
```

`-silent-allow` starts the proxy in allow-all mode and keeps it there until the next restart. There is no deny-all startup flag; use the REST API or the dashboard if you need deny at boot.

## Controlling it via the REST API

The same actions are available under `/api/allowall`:

```bash
# Check current state
curl http://localhost:43080/api/allowall

# Enable allow-all for 30 minutes
curl -X POST http://localhost:43080/api/allowall \
  -H 'Content-Type: application/json' \
  -d '{"mode": "allow", "duration": "30m"}'

# Enable deny-all until next restart
curl -X POST http://localhost:43080/api/allowall \
  -H 'Content-Type: application/json' \
  -d '{"mode": "deny", "duration": "restart"}'

# Disable silent mode immediately
curl -X DELETE http://localhost:43080/api/allowall
```

`duration` accepts any Go duration string (`5m`, `45m`, `2h30m`) up to a maximum of `8h`, or the literal string `"restart"` for "until next restart". `mode` is either `"allow"` or `"deny"`.

Calling `POST` while silent mode is already active resets the timer and lets you switch modes without first disabling.

## When to reach for something else

- If you want to teach greyproxy your allow list without being interrupted by prompts, use [learning mode](/greywall/learning-mode) in greywall instead; it watches traffic and generates rules rather than disabling policy entirely.
- If you want to silence desktop notifications without changing enforcement, toggle notifications in **Settings > Notifications** rather than using silent mode.
- If you want to block a specific destination rather than everything, add a deny rule from the **Rules** tab.
