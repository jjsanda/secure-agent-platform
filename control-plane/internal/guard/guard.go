// Package guard holds the policy checks the tool proxy applies to tool arguments
// before a tool runs: SSRF / private-address blocking for outbound URLs and
// path-traversal confinement for filesystem access. These are the authoritative,
// server-side enforcement (the worker applies the same checks defensively, but
// the proxy is the boundary that counts).
package guard

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// ErrBlocked marks a rejection by the policy guard. The tool proxy maps it to the
// POLICY_BLOCKED error code and audits it distinctly from an ordinary failure.
var ErrBlocked = errors.New("blocked by policy guard")

// CheckPublicURL validates that rawURL is an http(s) URL whose host resolves only
// to public, routable addresses — blocking loopback, private, link-local, and
// cloud-metadata targets (the classic SSRF vectors). It returns the resolved IPs
// so the caller can pin the connection to them and defeat DNS rebinding.
func CheckPublicURL(rawURL string) ([]net.IP, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed url", ErrBlocked)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q is not allowed", ErrBlocked, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: url has no host", ErrBlocked)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("%w: cannot resolve %q", ErrBlocked, host)
	}
	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return nil, fmt.Errorf("%w: %s resolves to a blocked address (%s)", ErrBlocked, host, ip)
		}
	}
	return ips, nil
}

// blockedCIDRs are ranges the standard net.IP helpers don't classify as private
// but which an agent must never reach: RFC 6598 CGNAT (used as internal fabric on
// some clouds) and IANA special-purpose blocks. Kept in sync with the worker's
// Python guard (agent-worker/.../guard/ssrf.py).
var blockedCIDRs = func() []*net.IPNet {
	out := make([]*net.IPNet, 0)
	for _, c := range []string{
		"100.64.0.0/10",   // RFC 6598 carrier-grade NAT
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // reserved (incl. 255.255.255.255 broadcast)
		"64:ff9b::/96",    // NAT64
		"2001:db8::/32",   // documentation
	} {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// IsBlockedIP reports whether ip is one an agent must never be able to reach.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	// Cloud instance-metadata endpoints (169.254.169.254 is already link-local,
	// but block it explicitly, along with the IPv6 variant).
	if ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("fd00:ec2::254")) {
		return true
	}
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ConfinePath cleans rawPath and guarantees the result stays within root,
// rejecting any parent-directory traversal or null-byte trick. It returns the
// confined absolute path.
func ConfinePath(root, rawPath string) (string, error) {
	if strings.ContainsRune(rawPath, 0) {
		return "", fmt.Errorf("%w: null byte in path", ErrBlocked)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(absRoot, rawPath)
	rel, err := filepath.Rel(absRoot, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes the sandbox root", ErrBlocked)
	}
	return joined, nil
}
