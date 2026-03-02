package greywallapi

import (
	"testing"
	"time"
)

func TestDNSCacheRegisterAndGet(t *testing.T) {
	cache := NewDNSCache()

	cache.RegisterIPs("example.com", []string{"1.2.3.4", "5.6.7.8"})

	if got := cache.GetCached("1.2.3.4"); got != "example.com" {
		t.Errorf("got %q, want %q", got, "example.com")
	}
	if got := cache.GetCached("5.6.7.8"); got != "example.com" {
		t.Errorf("got %q, want %q", got, "example.com")
	}
	if got := cache.GetCached("9.9.9.9"); got != "" {
		t.Errorf("expected empty for unknown IP, got %q", got)
	}
}

func TestDNSCacheExpiry(t *testing.T) {
	cache := &DNSCache{
		entries: make(map[string]dnsCacheEntry),
		ttl:     1 * time.Millisecond, // Very short TTL
	}

	cache.RegisterIPs("example.com", []string{"1.2.3.4"})

	// Should be cached immediately
	if got := cache.GetCached("1.2.3.4"); got != "example.com" {
		t.Errorf("expected cached value, got %q", got)
	}

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	if got := cache.GetCached("1.2.3.4"); got != "" {
		t.Errorf("expected expired entry to return empty, got %q", got)
	}
}

func TestDNSCacheResolveIPUsesCache(t *testing.T) {
	cache := NewDNSCache()

	// Pre-populate cache
	cache.RegisterIPs("example.com", []string{"1.2.3.4"})

	// ResolveIP should use cache
	got := cache.ResolveIP("1.2.3.4")
	if got != "example.com" {
		t.Errorf("expected cached result, got %q", got)
	}
}
