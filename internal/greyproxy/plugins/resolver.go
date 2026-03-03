package plugins

import (
	"context"
	"fmt"
	"net"

	"github.com/greyhavenhq/greyproxy/internal/gostcore/logger"
	"github.com/greyhavenhq/greyproxy/internal/gostcore/resolver"
	greyproxy "github.com/greyhavenhq/greyproxy/internal/greyproxy"
)

// Resolver implements resolver.Resolver.
// It resolves hostnames to IPs and populates the DNS cache.
type Resolver struct {
	cache *greyproxy.DNSCache
	log   logger.Logger
}

func NewResolver(cache *greyproxy.DNSCache) *Resolver {
	return &Resolver{
		cache: cache,
		log: logger.Default().WithFields(map[string]any{
			"kind":     "resolver",
			"resolver": "greyproxy",
		}),
	}
}

func (r *Resolver) Resolve(ctx context.Context, network, host string, opts ...resolver.Option) ([]net.IP, error) {
	r.log.Debugf("resolve: %s/%s", host, network)

	// Standard DNS resolution
	ips, err := net.DefaultResolver.LookupIP(ctx, networkToIPVersion(network), host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve %s: no results", host)
	}

	// Cache all resolved IPs -> hostname
	ipStrs := make([]string, len(ips))
	for i, ip := range ips {
		ipStrs[i] = ip.String()
	}
	r.cache.RegisterIPs(host, ipStrs)

	r.log.Debugf("resolved %s -> %v", host, ipStrs)
	return ips, nil
}

func networkToIPVersion(network string) string {
	switch network {
	case "ip4":
		return "ip4"
	case "ip6":
		return "ip6"
	default:
		return "ip"
	}
}
