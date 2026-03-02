package plugins

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-gost/core/bypass"
	"github.com/go-gost/core/logger"
	ctxvalue "github.com/go-gost/x/ctx"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
)

// Bypass implements bypass.Bypass.
// This is the main ACL enforcement point — it evaluates every destination
// against the rule database and decides whether to allow or block.
//
// IMPORTANT: In gost's blacklist mode (IsWhitelist()=false),
// Contains() returning true means BLOCK the connection.
type Bypass struct {
	db    *greywallapi.DB
	cache *greywallapi.DNSCache
	bus   *greywallapi.EventBus
	log   logger.Logger
}

func NewBypass(db *greywallapi.DB, cache *greywallapi.DNSCache, bus *greywallapi.EventBus) *Bypass {
	return &Bypass{
		db:    db,
		cache: cache,
		bus:   bus,
		log: logger.Default().WithFields(map[string]any{
			"kind":   "bypass",
			"bypass": "greywallapi",
		}),
	}
}

// IsWhitelist returns false — we operate in blacklist mode.
// Contains()=true means the address should be BLOCKED.
func (b *Bypass) IsWhitelist() bool {
	return false
}

// Contains evaluates the ACL for the given destination address.
// Returns true to BLOCK, false to ALLOW.
func (b *Bypass) Contains(ctx context.Context, network, addr string, opts ...bypass.Option) bool {
	start := time.Now()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		portStr = "0"
	}
	port, _ := strconv.Atoi(portStr)

	// Get client identity from context (set by auther)
	clientID := string(ctxvalue.ClientIDFromContext(ctx))
	containerName, containerID := resolveIdentity(clientID)

	// Resolve hostname
	resolvedHostname := b.resolveHostname(host)

	// Find matching rule
	matchHost := host
	if resolvedHostname != "" {
		matchHost = resolvedHostname
	}
	rule := greywallapi.FindMatchingRule(b.db, containerName, host, port, resolvedHostname)

	// Determine decision
	var allowed bool
	var ruleID *int64
	var deniedByRule bool

	if rule != nil {
		if rule.Action == "allow" {
			allowed = true
			ruleID = &rule.ID
		} else {
			allowed = false
			deniedByRule = true
			ruleID = &rule.ID
		}
	}
	// No rule = default deny (allowed stays false)

	// Log the request
	elapsed := time.Since(start).Milliseconds()
	result := "blocked"
	if allowed {
		result = "allowed"
	}

	go b.logRequest(containerName, containerID, host, port, resolvedHostname, result, ruleID, &elapsed)

	// Create/update pending request if blocked and not explicitly denied by rule
	if !allowed && !deniedByRule {
		go b.createPending(containerName, containerID, host, port, resolvedHostname)
	}

	if allowed {
		b.log.Debugf("ALLOW %s -> %s:%d (%s) rule=%v", containerName, matchHost, port, network, ruleID)
	} else {
		b.log.Debugf("BLOCK %s -> %s:%d (%s)", containerName, matchHost, port, network)
	}

	// In blacklist mode: return true to BLOCK
	return !allowed
}

func (b *Bypass) resolveHostname(host string) string {
	ip := net.ParseIP(host)
	if ip == nil {
		// It's already a hostname — register it for future IP lookups
		b.cache.RegisterHostname(host)
		return ""
	}
	// It's an IP — try to find the hostname
	return b.cache.ResolveIP(host)
}

func resolveIdentity(clientID string) (containerName, containerID string) {
	ip, username := ParseClientID(clientID)

	if username != "" && username != "proxy" {
		return username, ""
	}

	return fmt.Sprintf("unknown-%s", ip), ""
}

func (b *Bypass) logRequest(containerName, containerID, destHost string, destPort int, resolvedHostname, result string, ruleID *int64, responseTimeMs *int64) {
	_, err := greywallapi.CreateLogEntry(b.db, greywallapi.LogCreateInput{
		ContainerName:    containerName,
		ContainerID:      containerID,
		DestinationHost:  destHost,
		DestinationPort:  destPort,
		ResolvedHostname: resolvedHostname,
		Method:           "SOCKS5",
		Result:           result,
		RuleID:           ruleID,
		ResponseTimeMs:   responseTimeMs,
	})
	if err != nil {
		b.log.Warnf("failed to log request: %v", err)
	}
}

func (b *Bypass) createPending(containerName, containerID, destHost string, destPort int, resolvedHostname string) {
	pending, isNew, err := greywallapi.CreateOrUpdatePending(b.db, containerName, containerID, destHost, destPort, resolvedHostname)
	if err != nil {
		b.log.Warnf("failed to create pending: %v", err)
		return
	}

	if isNew {
		b.bus.Publish(greywallapi.Event{
			Type: greywallapi.EventPendingCreated,
			Data: pending.ToJSON(),
		})
	} else {
		b.bus.Publish(greywallapi.Event{
			Type: greywallapi.EventPendingUpdated,
			Data: pending.ToJSON(),
		})
	}
}
