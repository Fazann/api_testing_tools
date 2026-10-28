package httpclient

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
)

// blockedPrefixes are destinations that are almost never legitimate API test
// targets but are valuable to an attacker who controls an imported collection:
// cloud metadata services and other link-local addresses.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),     // IPv4 link-local, incl. 169.254.169.254 metadata
	netip.MustParsePrefix("fe80::/10"),          // IPv6 link-local
	netip.MustParsePrefix("fd00:ec2::/32"),      // AWS IPv6 metadata
	netip.MustParsePrefix("100.100.100.200/32"), // Alibaba Cloud metadata
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("::/128"),
}

var blockedHostnames = map[string]bool{
	"metadata.google.internal": true,
	"metadata":                 true,
	"instance-data":            true,
}

// BlockedError is returned when a request targets a sensitive destination.
type BlockedError struct {
	Target string
	Reason string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("request to %s blocked: %s (allow sensitive destinations in Settings if this is intended)", e.Target, e.Reason)
}

// CheckHost rejects well-known metadata hostnames and literal addresses in
// blocked ranges before any network activity happens.
func CheckHost(host string) error {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if blockedHostnames[h] {
		return &BlockedError{Target: host, Reason: "cloud metadata hostname"}
	}
	if addr, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return checkAddr(host, addr)
	}
	return nil
}

func checkAddr(target string, addr netip.Addr) error {
	addr = addr.Unmap()
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return &BlockedError{Target: target, Reason: "link-local or metadata address"}
		}
	}
	return nil
}

// dialControl runs after DNS resolution for every connection, so it also
// catches hostnames that resolve (or rebind) to a blocked address.
func dialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return checkAddr(address, addr)
}

// IsLocalHost reports whether host is a loopback name or address.
func IsLocalHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.IsLoopback()
	}
	return false
}
