package captchaweb

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies parses the comma-separated list of CAPTCHA_WEB_TRUSTED_PROXIES: each
// entry is an IP address (taken as a /32 or /128) or a CIDR. Spaces around entries are allowed
// and empty entries are skipped, so an empty string is an empty list. A bad entry is an error.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			prefix, err := netip.ParsePrefix(entry)
			if err != nil {
				return nil, fmt.Errorf("captchaweb: bad trusted proxy %q", entry)
			}
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("captchaweb: bad trusted proxy %q", entry)
		}
		addr = addr.WithZone("").Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

func inPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// parseHostAddr reads an address with an optional port, as RemoteAddr and a forwarding header
// entry spell it.
func parseHostAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().WithZone("").Unmap(), true
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.WithZone("").Unmap(), true
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr.WithZone("").Unmap(), true
		}
	}
	return netip.Addr{}, false
}

// sharedAddress reports whether addr cannot tell clients apart: a loopback, private,
// link-local or unspecified address is a proxy or a local peer, and every request would share it.
func sharedAddress(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified()
}

// ClientIP returns the address a per-IP budget is counted on and whether that address is
// usable as a bucket.
//
// The address is RemoteAddr, unless RemoteAddr is inside trusted: then the forwarding chain
// (X-Forwarded-For, all header lines as one list) is walked from the right to the first entry
// that is not inside trusted, because every hop to the right of an untrusted entry was added
// by a proxy of the owner's and everything to its left is client-controlled. A chain that is
// empty, all trusted or unparsable falls back to RemoteAddr, which is then a proxy address.
//
// bucket is false when the address is a proxy or a local peer that every request would share
// (loopback, private, link-local, or a trusted proxy itself): counting it would limit all
// members together, so the caller skips the per-IP window then.
//
// No header other than X-Forwarded-For is ever read.
func ClientIP(r *http.Request, trusted []netip.Prefix) (addr netip.Addr, bucket bool) {
	peer, ok := parseHostAddr(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	if !inPrefixes(peer, trusted) {
		return peer, !sharedAddress(peer)
	}

	var chain []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		chain = append(chain, strings.Split(line, ",")...)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		entry, ok := parseHostAddr(chain[i])
		if !ok {
			return peer, false
		}
		if inPrefixes(entry, trusted) {
			continue
		}
		return entry, true
	}
	return peer, false
}
