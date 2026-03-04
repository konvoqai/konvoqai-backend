package controller

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var blockedOutboundHosts = map[string]struct{}{
	"localhost":                {},
	"localhost.localdomain":    {},
	"metadata.google.internal": {},
}

var blockedOutboundNetworks = mustParseIPPrefixes([]string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
})

func mustParseIPPrefixes(values []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			continue
		}
		out = append(out, prefix)
	}
	return out
}

func isBlockedIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	for _, prefix := range blockedOutboundNetworks {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func resolveAndValidateHost(ctx context.Context, hostname string) error {
	host := strings.ToLower(strings.TrimSpace(hostname))
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if _, blocked := blockedOutboundHosts[host]; blocked {
		return fmt.Errorf("host is not allowed")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("ip is not allowed")
		}
		return nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("dns resolution failed")
	}
	if len(addrs) == 0 {
		return fmt.Errorf("no dns records found")
	}
	for _, addr := range addrs {
		if isBlockedIP(addr.IP) {
			return fmt.Errorf("host resolves to private or blocked ip")
		}
	}
	return nil
}

func validatePort(u *url.URL) error {
	port := strings.TrimSpace(u.Port())
	if port == "" {
		return nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return fmt.Errorf("invalid port")
	}
	return nil
}

func (c *Controller) validateOutboundURL(raw string, allowHTTP bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid url")
	}
	if u.User != nil {
		return nil, fmt.Errorf("url user info is not allowed")
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "https":
	case "http":
		if !allowHTTP {
			return nil, fmt.Errorf("http urls are not allowed")
		}
	default:
		return nil, fmt.Errorf("unsupported url scheme")
	}
	if err := validatePort(u); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := resolveAndValidateHost(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

func (c *Controller) validateScrapeTarget(raw string) (*url.URL, error) {
	return c.validateOutboundURL(raw, true)
}

func (c *Controller) validateWebhookTarget(raw string) (*url.URL, error) {
	allowHTTP := !c.cfg.IsProduction
	return c.validateOutboundURL(raw, allowHTTP)
}
