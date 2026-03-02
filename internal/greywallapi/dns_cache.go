package greywallapi

import (
	"context"
	"net"
	"sync"
	"time"
)

const defaultDNSCacheTTL = 1 * time.Hour

type dnsCacheEntry struct {
	hostname string
	expiry   time.Time
}

// DNSCache provides a thread-safe IP-to-hostname cache with TTL.
type DNSCache struct {
	mu      sync.RWMutex
	entries map[string]dnsCacheEntry // IP -> hostname
	ttl     time.Duration
}

func NewDNSCache() *DNSCache {
	return &DNSCache{
		entries: make(map[string]dnsCacheEntry),
		ttl:     defaultDNSCacheTTL,
	}
}

// ResolveIP attempts to get a hostname for the given IP.
// Checks cache first, then falls back to reverse DNS lookup.
func (c *DNSCache) ResolveIP(ip string) string {
	// Check cache
	if hostname := c.GetCached(ip); hostname != "" {
		return hostname
	}

	// Try reverse DNS
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}

	hostname := names[0]
	// Remove trailing dot from DNS names
	if len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1]
	}

	c.mu.Lock()
	c.entries[ip] = dnsCacheEntry{hostname: hostname, expiry: time.Now().Add(c.ttl)}
	c.mu.Unlock()

	return hostname
}

// RegisterHostname does a forward DNS lookup and caches all resulting IPs.
func (c *DNSCache) RegisterHostname(hostname string) {
	ips, err := net.DefaultResolver.LookupHost(context.Background(), hostname)
	if err != nil {
		return
	}
	c.RegisterIPs(hostname, ips)
}

// RegisterIPs pre-populates the cache with known IP -> hostname mappings.
func (c *DNSCache) RegisterIPs(hostname string, ips []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiry := time.Now().Add(c.ttl)
	for _, ip := range ips {
		c.entries[ip] = dnsCacheEntry{hostname: hostname, expiry: expiry}
	}
}

// GetCached returns the cached hostname for an IP, or empty string if not found/expired.
func (c *DNSCache) GetCached(ip string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[ip]
	if !ok || time.Now().After(entry.expiry) {
		return ""
	}
	return entry.hostname
}
