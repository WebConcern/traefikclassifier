package lib

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func classifyIP(c *Classifier, ip string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, ip, "")
	return req
}

// rewriteFile replaces a file's content and moves its mtime forward so the
// refresher sees a change even on filesystems with coarse timestamps.
func rewriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

func TestNewClassifierMissingFileContinues(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "203.0.113.10\n")
	c := mustNewClassifier(t, &Config{
		DatacenterFile: filepath.Join(t.TempDir(), "does-not-exist.csv"),
		TorFile:        torFile,
	})

	assertHeaderVal(t, classifyIP(c, "203.0.113.10"), TrafficTorHeader, "true")
}

func TestClassifyVPNBareIPv6IsSingleHost(t *testing.T) {
	vpnFile := writeTempFile(t, "vpn.txt", "2001:db8::1\n")
	c := mustNewClassifier(t, &Config{VPNFile: vpnFile})

	assertHeaderVal(t, classifyIP(c, "2001:db8::1"), TrafficVPNHeader, "true")
	assertHeaderVal(t, classifyIP(c, "2001:db8:ffff::1"), TrafficVPNHeader, "false")
}

func TestClassifyVPNRangeBoundaries(t *testing.T) {
	// Overlapping, adjacent and nested ranges, plus a bare IPv4 address.
	vpnFile := writeTempFile(t, "vpn.txt", strings.Join([]string{
		"10.0.0.0/24",
		"10.0.1.0/24",
		"10.0.0.128/25",
		"172.16.0.0/12",
		"172.20.5.0/24",
		"192.0.2.7",
		"2001:db8::/32",
	}, "\n"))
	c := mustNewClassifier(t, &Config{VPNFile: vpnFile})

	tests := []struct {
		ip   string
		want string
	}{
		{"9.255.255.255", "false"},
		{"10.0.0.0", "true"},
		{"10.0.1.255", "true"},
		{"10.0.2.0", "false"},
		{"172.15.255.255", "false"},
		{"172.16.0.0", "true"},
		{"172.31.255.255", "true"},
		{"172.32.0.0", "false"},
		{"192.0.2.6", "false"},
		{"192.0.2.7", "true"},
		{"192.0.2.8", "false"},
		{"2001:db8:1234::1", "true"},
		{"2001:db9::1", "false"},
		{"not-an-ip", "false"},
	}
	for _, tt := range tests {
		if got := classifyIP(c, tt.ip).Header.Get(TrafficVPNHeader); got != tt.want {
			t.Errorf("ip=%s: got X-Traffic-VPN=%q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestClassifyTorMatchesNonCanonicalIPv6(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "2001:DB8:0:0::0001\n")
	c := mustNewClassifier(t, &Config{TorFile: torFile})

	assertHeaderVal(t, classifyIP(c, "2001:db8::1"), TrafficTorHeader, "true")
}

func TestRefreshPicksUpChangedFile(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "203.0.113.10\n")
	c := mustNewClassifier(t, &Config{TorFile: torFile})
	assertHeaderVal(t, classifyIP(c, "203.0.113.20"), TrafficTorHeader, "false")

	rewriteFile(t, torFile, "203.0.113.20\n")
	RefreshFiles()

	assertHeaderVal(t, classifyIP(c, "203.0.113.20"), TrafficTorHeader, "true")
	assertHeaderVal(t, classifyIP(c, "203.0.113.10"), TrafficTorHeader, "false")
}

func TestRefreshLoadsFileCreatedAfterStartup(t *testing.T) {
	torFile := filepath.Join(t.TempDir(), "tor.txt")
	c := mustNewClassifier(t, &Config{TorFile: torFile})
	assertHeaderVal(t, classifyIP(c, "203.0.113.10"), TrafficTorHeader, "false")

	rewriteFile(t, torFile, "203.0.113.10\n")
	RefreshFiles()

	assertHeaderVal(t, classifyIP(c, "203.0.113.10"), TrafficTorHeader, "true")
}

func TestRefreshKeepsDataWhenFileRemoved(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "203.0.113.10\n")
	c := mustNewClassifier(t, &Config{TorFile: torFile})

	if err := os.Remove(torFile); err != nil {
		t.Fatal(err)
	}
	RefreshFiles()

	assertHeaderVal(t, classifyIP(c, "203.0.113.10"), TrafficTorHeader, "true")
}

func TestRefreshRejectsEmptyList(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "203.0.113.10\n")
	c := mustNewClassifier(t, &Config{TorFile: torFile})

	rewriteFile(t, torFile, "")
	RefreshFiles()

	assertHeaderVal(t, classifyIP(c, "203.0.113.10"), TrafficTorHeader, "true")
}

func TestLoadTorExitsRejectsOverlongLine(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "203.0.113.10\n"+strings.Repeat("x", 70*1024)+"\n")

	if _, err := loadTorExits(torFile); err == nil {
		t.Fatal("expected an error for a line longer than the scanner buffer")
	}
}

func BenchmarkClassifyVPN10k(b *testing.B) {
	lines := make([]string, 0, 10000)
	for i := 0; i < 10000; i++ {
		lines = append(lines, fmt.Sprintf("%d.%d.%d.0/24", 11+i/65536, i/256%256, i%256))
	}
	path := filepath.Join(b.TempDir(), "vpn.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		b.Fatal(err)
	}
	ResetClassifier()
	c, err := NewClassifier(&Config{VPNFile: path})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyIP(c, "188.193.88.199") // not in the list: the old linear scan's worst case
	}
}
