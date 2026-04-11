---
id: tls-interception
title: TLS Interception (MITM)
---

# TLS Interception (MITM)

Most modern APIs use HTTPS, which means greyproxy can only see encrypted bytes unless it actively decrypts the TLS stream. TLS interception (also called MITM, for man-in-the-middle) is what lets greyproxy read HTTPS request and response contents, and it is required for almost every advanced feature: [credential substitution](./credentials), [LLM conversation tracking](./conversations), and [header redaction](./header-redaction) of HTTPS traffic all depend on it.

When MITM is off, greyproxy still proxies HTTPS correctly, but it can only see the connection metadata (destination host via SNI, port, timing). Request and response bodies, headers, and authentication tokens remain opaque.

## How it works

Greyproxy ships its own CA certificate. When an HTTPS request goes through the proxy:

1. The client opens a `CONNECT` tunnel to greyproxy.
2. Instead of blindly piping bytes, greyproxy terminates the TLS connection itself, using a certificate signed on the fly by the greyproxy CA for the target hostname.
3. Greyproxy opens a second TLS connection to the real upstream and forwards decrypted traffic between the two.
4. Because the client trusts the greyproxy CA (after you install it), the forged certificate validates and no warnings are shown.

The CA key and certificate are ECDSA P-256 and valid for ten years. They are stored in greyproxy's data directory:

- macOS: `~/Library/Application Support/greyproxy/ca-cert.pem` and `ca-key.pem`
- Linux: `~/.local/share/greyproxy/ca-cert.pem` and `ca-key.pem`

You can override this location with `GREYPROXY_DATA_HOME` or `XDG_DATA_HOME`.

## Generating and trusting the CA

Greyproxy exposes a `cert` subcommand that handles the full lifecycle:

```bash
# Generate the CA key and certificate (run once, per machine)
greyproxy cert generate

# Add the CA to the OS trust store (requires sudo)
greyproxy cert install

# Remove the CA from the OS trust store
greyproxy cert uninstall

# Reload the CA in a running greyproxy without restarting
greyproxy cert reload
```

`generate` refuses to overwrite an existing CA; pass `-f` to force regeneration. After regenerating you should re-run `install` (the old cert is no longer valid) and `cert reload` so the running proxy picks up the new key.

### What `install` does

- **macOS**: imports the cert into `/Library/Keychains/System.keychain` via `security add-trusted-cert` and marks it as a trusted SSL root.
- **Debian/Ubuntu**: copies the cert to `/usr/local/share/ca-certificates/greyproxy-ca.crt` and runs `update-ca-certificates`.
- **Fedora, RHEL, Arch, openSUSE**: copies the cert to `/etc/ca-certificates/trust-source/anchors/greyproxy-ca.crt` and runs `update-ca-trust`.

All of these require root, so the command will prompt for sudo.

### Trusting the CA inside a sandbox

System trust stores cover most tools, but some runtimes (Node.js, Python's `certifi`, Go, curl with a custom bundle) maintain their own CA lists. When you run a tool inside greywall and it fails with a TLS verification error, you usually need to point that tool at the greyproxy CA explicitly, for example:

```bash
export NODE_EXTRA_CA_CERTS=~/.local/share/greyproxy/ca-cert.pem
export REQUESTS_CA_BUNDLE=~/.local/share/greyproxy/ca-cert.pem
export SSL_CERT_FILE=~/.local/share/greyproxy/ca-cert.pem
```

Greywall's default profiles set these variables automatically for well-known tools; see the [Greywall credential protection guide](/greywall/credential-protection) for the full list.

## Enabling and disabling MITM

MITM is **enabled by default**. The setting is stored under the `mitmEnabled` key in `settings.json` inside the greyproxy data directory.

### From the dashboard

Open **Settings**, scroll to **HTTPS Inspection**, and toggle **Enable TLS Interception**. Changes apply immediately without restarting the proxy.

### From the REST API

```bash
# Turn MITM on
curl -X PUT http://localhost:43080/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"mitmEnabled": true}'

# Turn MITM off (HTTPS will be passed through as opaque CONNECT tunnels)
curl -X PUT http://localhost:43080/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"mitmEnabled": false}'
```

## Bypassing specific hosts

Some endpoints should never be intercepted: banking sites, hosts that use certificate pinning, or internal services where you trust the transport layer already. Greyproxy supports a per-host bypass list that is checked before interception; matching connections are proxied as plain CONNECT tunnels. Bypass rules accept glob patterns on hostnames and CIDR ranges on IPs, and you can manage them from **Settings > HTTPS Inspection > Bypass rules** or via the settings API.

When a connection is bypassed, its activity row is tagged with the reason `mitm_bypass`. Connections that are passed through because MITM is globally off are tagged `mitm_disabled`.

## Gotchas

- **Certificate pinning defeats MITM.** Any client that pins specific upstream certificates (mobile SDKs, some desktop apps, a handful of CLI tools) will refuse the forged certificate and fail. Add pinned hosts to the bypass list.
- **Regenerating the CA invalidates existing trust.** After `greyproxy cert generate -f` you must re-run `greyproxy cert install` on every machine that trusted the previous CA, otherwise HTTPS will break.
- **The CA key is sensitive.** Anyone who obtains `ca-key.pem` from a machine where it is trusted can forge TLS certificates for that machine. Keep the data directory restricted to the user running greyproxy.
- **Disabling MITM does not stop traffic.** HTTPS still flows; you just lose visibility, credential substitution, and conversation decoding. Use the dashboard Activity view to confirm which mode each transaction was handled in.
