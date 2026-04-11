---
id: troubleshooting
title: Troubleshooting
---

# Greyproxy Troubleshooting

## Service Won't Start

### Check the service status

```bash
greyproxy service status

# Linux: view logs
journalctl --user -u greyproxy -f

# macOS: view logs
tail -f ~/Library/Logs/greyproxy.log
```

### Port already in use

If another process is using port `43080`, `43051`, `43052`, or `43053`, greyproxy will fail to start.

```bash
# Linux/macOS: find what's using a port
lsof -i :43080
```

Either stop the conflicting process, or run greyproxy with a custom config on different ports:

```bash
greyproxy serve -C ~/greyproxy-custom.yml
```

See [Configuration](./configuration) for port settings.

### Binary not found after `greyproxy install`

`install` copies the binary to `~/.local/bin/greyproxy`. Make sure that directory is on your PATH:

```bash
echo $PATH | grep -o '\.local/bin'
# If empty, add it:
export PATH="$HOME/.local/bin:$PATH"
```

Add to your shell profile (`.bashrc`, `.zshrc`, etc.) to make it permanent.

---

## Dashboard Not Loading

### Check greyproxy is running

```bash
greyproxy service status
# or
curl http://localhost:43080/api/health
```

If it returns `{"status":"ok"}`, the service is up. If not:

```bash
greyproxy service start
```

### CORS or browser security errors

The dashboard is served over plain HTTP on localhost (intentional, since it is a local tool). If your browser is blocking it, try:

- Using `http://` not `https://` in the URL
- Disabling HTTPS-only mode for localhost in your browser settings

### Dashboard loads but shows no data

This is normal on a fresh install, since there are no rules or traffic yet. Start routing traffic through greyproxy (via greywall or by setting `ALL_PROXY=socks5://localhost:43052`) and make some network requests.

---

## Traffic Not Being Routed Through Greyproxy

### Verify greywall is configured to use greyproxy

Check your greywall config (`~/.config/greywall/greywall.json`):

```json
{
  "network": {
    "proxyUrl": "socks5://localhost:43052",
    "dnsAddr": "localhost:43053"
  }
}
```

Run `greywall check` to verify greyproxy is detected and running.

### Verify proxy settings manually

```bash
# Test SOCKS5 proxy directly
curl --proxy socks5h://localhost:43052 https://example.com

# Test HTTP proxy
curl --proxy http://localhost:43051 https://example.com
```

If these fail but greyproxy is running, the port might be blocked by a local firewall.

### On macOS: some traffic bypasses the proxy

On macOS, greywall sets `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY` environment variables. Applications that don't respect proxy environment variables (certain native binaries, some Go programs with direct socket calls) will bypass greyproxy.

This is a macOS limitation; there is no user-accessible network namespace. On Linux, greywall uses a TUN device to capture all traffic regardless of application behavior.

---

## Rules Not Taking Effect

### Rule was added but traffic still blocked

Rules take effect immediately without restarting greyproxy. If traffic is still blocked after adding an allow rule, check:

1. The rule pattern actually matches the destination (see the [pattern syntax guide](./rules#pattern-syntax)).
2. There is no deny rule that matches first (deny rules are checked before allow rules).
3. The destination hostname is correct; check the Activity tab in the dashboard for the exact hostname being requested.

```bash
# Check what rules currently exist
curl http://localhost:43080/api/rules | jq .
```

### DNS name not resolving

If you're routing DNS through greyproxy (`localhost:43053`) and DNS lookups are failing:

```bash
# Test DNS resolution through greyproxy
dig @localhost -p 43053 example.com
```

If DNS fails but the proxy itself is up, check whether the upstream DNS server is reachable from the host.

---

## HTTPS Interception Problems

### "Certificate not trusted" errors in the browser or CLI tools

HTTPS inspection requires the greyproxy CA to be trusted by the operating system. `greyproxy install` takes care of this automatically; if you built from source or used Homebrew, you may need to run it manually:

```bash
greyproxy cert generate
greyproxy cert install
```

On Linux, this writes the CA to `/etc/ca-certificates/trust-source/anchors/` (Arch, Fedora, RHEL) or `/usr/local/share/ca-certificates/` (Debian, Ubuntu) and runs the matching `update-ca-trust` or `update-ca-certificates` command. On macOS, it adds the CA to the System keychain.

Some applications maintain their own trust store (for example, Firefox, and Python's `certifi` bundle). For those you may need to import `ca-cert.pem` from greyproxy's data directory into the application-specific store as well.

### Conversations or transactions are empty

Conversation tracking and transaction capture both require MITM to be active. Check:

```bash
curl http://localhost:43080/api/settings | jq .mitmEnabled
curl http://localhost:43080/api/cert/status
```

If MITM is disabled, re-enable it from the dashboard settings or with:

```bash
curl -X PUT http://localhost:43080/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"mitmEnabled": true}'
```

### Regenerating the CA

If the CA keypair is lost or you want to rotate it:

```bash
greyproxy cert generate -f    # overwrite existing keypair
greyproxy cert install -f     # re-trust in the OS store
greyproxy cert reload         # tell the running greyproxy to pick it up
```

`cert reload` avoids having to restart the service.

---

## Database Issues

### `greyproxy.db` is corrupted or missing

The database is created automatically on first start. If it is corrupted:

```bash
# Stop greyproxy
greyproxy service stop

# Remove the database (rules, logs, conversations, and transactions will be lost)
rm ~/.local/share/greyproxy/greyproxy.db                   # Linux
rm ~/Library/Application\ Support/greyproxy/greyproxy.db   # macOS

# Restart, a fresh database will be created
greyproxy service start
```

### Database location

The default database path is `greyproxy.db` inside greyproxy's data directory:

| Platform | Default |
|----------|---------|
| Linux | `~/.local/share/greyproxy/greyproxy.db` |
| macOS | `~/Library/Application Support/greyproxy/greyproxy.db` |

Override with the `db` field in your config:

```yaml
greyproxy:
  db: "/var/lib/greyproxy/greyproxy.db"
```

---

## Logs and Diagnostics

### View request logs

```bash
# Via API
curl http://localhost:43080/api/logs | jq .

# Via dashboard
# Open http://localhost:43080 → Logs tab
```

### Enable verbose logging

Run in foreground with debug output:

```bash
greyproxy serve -C greyproxy.yml  # runs in foreground, logs to stdout
```

### Check greywall → greyproxy integration

```bash
# Verify greywall can see greyproxy
greywall check

# Run a command with monitor mode to see proxy decisions
greywall -m -- curl https://example.com
```

---

## Getting Help

If you're still stuck, [open an issue on GitHub](https://github.com/GreyhavenHQ/greyproxy/issues) with:

- Output of `greyproxy service status`
- Output of `greywall check` (if applicable)
- The request logs from `curl http://localhost:43080/api/logs`
- Your greyproxy config (with any sensitive values removed)
