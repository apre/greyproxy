package plugins

import (
	"context"
	"net"
	"strings"
	"sync"

	"github.com/go-gost/core/handler"
	"github.com/go-gost/core/hop"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
	"github.com/go-gost/x/registry"
	"github.com/miekg/dns"
)

var (
	sharedDNSCache   *greywallapi.DNSCache
	sharedDNSCacheMu sync.RWMutex
)

// SetSharedDNSCache sets the DNS cache used by the handler wrapper.
// Called from buildGreywallApiService() after the cache is created.
func SetSharedDNSCache(cache *greywallapi.DNSCache) {
	sharedDNSCacheMu.Lock()
	defer sharedDNSCacheMu.Unlock()
	sharedDNSCache = cache
}

func getSharedDNSCache() *greywallapi.DNSCache {
	sharedDNSCacheMu.RLock()
	defer sharedDNSCacheMu.RUnlock()
	return sharedDNSCache
}

// OverrideDNSHandler wraps the registered "dns" handler with cache population.
// Must be called BEFORE loader.Load(cfg) so services use the wrapped handler.
func OverrideDNSHandler() {
	origFactory := registry.HandlerRegistry().Get("dns")
	if origFactory == nil {
		return
	}

	// Must unregister first — the registry uses LoadOrStore and calls Fatal on duplicates.
	registry.HandlerRegistry().Unregister("dns")

	registry.HandlerRegistry().Register("dns", func(opts ...handler.Option) handler.Handler {
		inner := origFactory(opts...)
		return &cachingDNSHandler{inner: inner}
	})
}

// cachingDNSHandler wraps a DNS handler to intercept responses and populate the DNS cache.
type cachingDNSHandler struct {
	inner handler.Handler
}

func (h *cachingDNSHandler) Init(metadata md.Metadata) error {
	return h.inner.Init(metadata)
}

func (h *cachingDNSHandler) Handle(ctx context.Context, conn net.Conn, opts ...handler.HandleOption) error {
	cache := getSharedDNSCache()
	if cache == nil {
		// Cache not yet initialized, pass through without interception
		return h.inner.Handle(ctx, conn, opts...)
	}
	wrapped := &dnsResponseCapture{Conn: conn, cache: cache}
	return h.inner.Handle(ctx, wrapped, opts...)
}

// Forward implements handler.Forwarder for the wrapped handler.
func (h *cachingDNSHandler) Forward(hop hop.Hop) {
	if f, ok := h.inner.(handler.Forwarder); ok {
		f.Forward(hop)
	}
}

// dnsResponseCapture wraps net.Conn to intercept DNS query (Read) and response (Write).
type dnsResponseCapture struct {
	net.Conn
	cache *greywallapi.DNSCache
	query []byte
}

func (c *dnsResponseCapture) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err == nil && n > 0 {
		c.query = make([]byte, n)
		copy(c.query, b[:n])
	}
	return n, err
}

func (c *dnsResponseCapture) Write(b []byte) (int, error) {
	if c.query != nil && len(b) > 0 {
		c.populateCache(b)
	}
	return c.Conn.Write(b)
}

func (c *dnsResponseCapture) populateCache(response []byte) {
	log := logger.Default().WithFields(map[string]any{
		"kind":   "dns-cache",
		"module": "greywallapi",
	})

	mr := &dns.Msg{}
	if err := mr.Unpack(response); err != nil {
		return
	}

	if len(mr.Question) == 0 {
		return
	}

	hostname := strings.TrimSuffix(mr.Question[0].Name, ".")
	if hostname == "" {
		return
	}

	var ips []string
	for _, rr := range mr.Answer {
		switch a := rr.(type) {
		case *dns.A:
			ips = append(ips, a.A.String())
		case *dns.AAAA:
			ips = append(ips, a.AAAA.String())
		}
	}

	if len(ips) > 0 {
		c.cache.RegisterIPs(hostname, ips)
		log.Debugf("cached %s -> %v", hostname, ips)
	}
}
