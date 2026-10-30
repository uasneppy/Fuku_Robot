//go:build testtools

package captchaweb

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	mustTrusted := func(s string) []netip.Prefix {
		t.Helper()
		prefixes, err := ParseTrustedProxies(s)
		if err != nil {
			t.Fatalf("ParseTrustedProxies(%q) error = %v", s, err)
		}
		return prefixes
	}

	cases := []struct {
		name       string
		remote     string
		headers    map[string][]string
		trusted    string
		wantAddr   string
		wantBucket bool
	}{
		{
			name:       "no list: a forwarding header is ignored",
			remote:     "203.0.113.9:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"1.2.3.4"}},
			wantAddr:   "203.0.113.9",
			wantBucket: true,
		},
		{name: "no list: a loopback peer has no usable address", remote: "127.0.0.1:4000", wantAddr: "127.0.0.1", wantBucket: false},
		{name: "no list: a private peer has no usable address", remote: "10.1.2.3:4000", wantAddr: "10.1.2.3", wantBucket: false},
		{
			name:       "trusted peer: the rightmost untrusted address wins",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"198.51.100.7, 10.0.0.9"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "198.51.100.7",
			wantBucket: true,
		},
		{
			name:       "trusted peer: an address forged on the left is never reached",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"6.6.6.6, 198.51.100.7, 10.0.0.9"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "198.51.100.7",
			wantBucket: true,
		},
		{
			name:       "trusted peer: several header lines are one chain",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"6.6.6.6", "198.51.100.7, 10.0.0.9"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "198.51.100.7",
			wantBucket: true,
		},
		{
			name:       "trusted peer: a shared private entry from the chain is no bucket",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"172.16.0.7"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "172.16.0.7",
			wantBucket: false,
		},
		{
			name:       "trusted peer: a loopback entry from the chain is no bucket",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"198.51.100.7, 127.0.0.1"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "127.0.0.1",
			wantBucket: false,
		},
		{
			name:       "trusted peer: a chain that is all trusted falls back to the peer",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"10.0.0.7, 10.0.0.9"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "10.0.0.5",
			wantBucket: false,
		},
		{name: "trusted peer without a header falls back to the peer", remote: "10.0.0.5:4000", trusted: "10.0.0.0/8", wantAddr: "10.0.0.5", wantBucket: false},
		{
			name:       "trusted peer: an unparsable entry falls back to the peer",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"not-an-ip, 10.0.0.9"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "10.0.0.5",
			wantBucket: false,
		},
		{
			name:       "untrusted peer: the header is ignored even with a list",
			remote:     "203.0.113.9:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"1.2.3.4"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "203.0.113.9",
			wantBucket: true,
		},
		{
			name:       "other proxy headers are never read",
			remote:     "203.0.113.9:4000",
			headers:    map[string][]string{"X-Real-Ip": {"1.2.3.4"}, "Cf-Connecting-Ip": {"5.6.7.8"}},
			wantAddr:   "203.0.113.9",
			wantBucket: true,
		},
		{
			name:       "other proxy headers are never read behind a trusted proxy",
			remote:     "10.0.0.5:4000",
			headers:    map[string][]string{"X-Real-Ip": {"1.2.3.4"}, "Cf-Connecting-Ip": {"5.6.7.8"}},
			trusted:    "10.0.0.0/8",
			wantAddr:   "10.0.0.5",
			wantBucket: false,
		},
		{name: "an IPv6 peer works", remote: "[2001:db8::1]:4000", wantAddr: "2001:db8::1", wantBucket: true},
		{
			name:       "an IPv6 proxy chain works",
			remote:     "[fd00::5]:4000",
			headers:    map[string][]string{"X-Forwarded-For": {"2001:db8::7, fd00::9"}},
			trusted:    "fd00::/8",
			wantAddr:   "2001:db8::7",
			wantBucket: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/captcha/verify", nil)
			req.RemoteAddr = tc.remote
			for k, values := range tc.headers {
				for _, v := range values {
					req.Header.Add(k, v)
				}
			}
			addr, bucket := ClientIP(req, mustTrusted(tc.trusted))
			if addr.String() != tc.wantAddr || bucket != tc.wantBucket {
				t.Fatalf("ClientIP() = (%v, %v), want (%s, %v)", addr, bucket, tc.wantAddr, tc.wantBucket)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies("10.0.0.0/8, 192.168.1.10, ::1")
	if err != nil {
		t.Fatalf("ParseTrustedProxies() error = %v", err)
	}
	want := []string{"10.0.0.0/8", "192.168.1.10/32", "::1/128"}
	if len(got) != len(want) {
		t.Fatalf("ParseTrustedProxies() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("prefix %d = %v, want %s", i, got[i], want[i])
		}
	}

	if got, err := ParseTrustedProxies(""); err != nil || len(got) != 0 {
		t.Fatalf("ParseTrustedProxies(\"\") = (%v, %v), want an empty list", got, err)
	}
	if _, err := ParseTrustedProxies("10.0.0.0/8, nope"); err == nil {
		t.Fatalf("ParseTrustedProxies() accepted a bad entry")
	}
}
