# Greyproxy

A managed network proxy with a built-in web dashboard, rule engine, and REST API. Greyproxy wraps powerful multi-protocol tunneling capabilities with an intuitive management layer for controlling and monitoring network traffic.

## Features

- **Web Dashboard**: Real-time overview of proxy traffic, pending requests, and rule management, all served from a single binary
- **Rule Engine**: Define allow/deny rules with pattern matching on container, destination, and port
- **Pending Requests**: Review and approve/deny network requests awaiting a policy decision
- **Multi-Protocol Proxy**: HTTP, SOCKS5, and DNS proxies with forwarding chain support
- **DNS Caching**: Built-in DNS resolution and caching with hostname enrichment on requests
- **REST API**: Full HTTP API for automation and integration
- **Real-Time Updates**: WebSocket-based live updates on the dashboard
- **Single Binary**: Web UI, fonts, icons, and assets are all embedded, no separate frontend to deploy

## Quick Start

### From Source

```bash
git clone https://github.com/greyhavenhq/greyproxy.git
cd greyproxy/cmd/greyproxy
go build
```

### Configuration

Greyproxy uses a YAML configuration file. See [`greyproxy.yml`](greyproxy.yml) for a full example.

```yaml
greyproxy:
  addr: ":43080"        # Dashboard and API
  db: "./greyproxy.db"  # SQLite database

services:
  - name: http-proxy
    addr: ":43051"
    handler:
      type: http
    listener:
      type: tcp

  - name: socks5-proxy
    addr: ":43052"
    handler:
      type: socks5
    listener:
      type: tcp
```

### Run

```bash
./greyproxy -C greyproxy.yml
```

The dashboard will be available at `http://localhost:43080`.

## Default Ports

| Service       | Port    |
|---------------|---------|
| Dashboard/API | `43080` |
| HTTP Proxy    | `43051` |
| SOCKS5 Proxy  | `43052` |
| DNS Proxy     | `43053` |

## Acknowledgments

Greyproxy is a fork of [**GOST** (GO Simple Tunnel)](https://github.com/go-gost/gost) by [ginuerzh](https://github.com/ginuerzh). GOST is an excellent and feature-rich tunnel and proxy toolkit written in Go, and its solid foundation made this project possible. We are grateful to ginuerzh and all GOST contributors for their work and for releasing it under the MIT license.

The core tunneling, protocol handling, and proxy infrastructure in Greyproxy originates from GOST v3. For documentation on the underlying proxy and tunnel capabilities, refer to the [GOST documentation](https://gost.run/en/).

## License

This project is licensed under the [MIT License](LICENSE), the same license as the original GOST project.

Copyright (c) 2026 The Greyproxy Authors. Original copyright (c) 2016 ginuerzh.
