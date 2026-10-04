package guard

import (
	"errors"
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":         false, // public
		"1.1.1.1":         false, // public
		"127.0.0.1":       true,  // loopback
		"10.0.0.1":        true,  // private
		"192.168.1.1":     true,  // private
		"172.16.5.4":      true,  // private
		"169.254.169.254": true,  // metadata / link-local
		"100.64.0.1":      true,  // CGNAT (RFC 6598)
		"198.18.0.1":      true,  // benchmarking range
		"240.0.0.1":       true,  // reserved
		"0.0.0.0":         true,  // unspecified
		"::1":             true,  // loopback v6
		"fc00::1":         true,  // ULA (private v6)
		"2606:4700::1111": false, // public v6
	}
	for ipStr, want := range cases {
		if got := IsBlockedIP(net.ParseIP(ipStr)); got != want {
			t.Errorf("IsBlockedIP(%s) = %v, want %v", ipStr, got, want)
		}
	}
}

func TestCheckPublicURL_Blocks(t *testing.T) {
	// Hosts given as IP literals resolve locally (no external DNS needed).
	blocked := []string{
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.1.2.3/",
		"http://192.168.0.1/",
		"http://[::1]/",
		"ftp://example.com/",
		"file:///etc/passwd",
		"http:///no-host",
		"::not-a-url",
	}
	for _, u := range blocked {
		if _, err := CheckPublicURL(u); !errors.Is(err, ErrBlocked) {
			t.Errorf("CheckPublicURL(%q) should be blocked, got err=%v", u, err)
		}
	}
}

func TestConfinePath(t *testing.T) {
	root := "/srv/sandbox"
	ok := []string{"readme.md", "sub/dir/doc.txt", "./notes.md"}
	for _, p := range ok {
		if _, err := ConfinePath(root, p); err != nil {
			t.Errorf("ConfinePath(%q) should be allowed, got %v", p, err)
		}
	}
	bad := []string{"../etc/passwd", "a/../../b", "sub/../../escape", "../../"}
	for _, p := range bad {
		if _, err := ConfinePath(root, p); !errors.Is(err, ErrBlocked) {
			t.Errorf("ConfinePath(%q) should be blocked, got %v", p, err)
		}
	}
}
