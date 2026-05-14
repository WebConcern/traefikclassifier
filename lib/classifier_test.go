package lib

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClassifyDatacenter(t *testing.T) {
	asnFile := writeTempFile(t, "asn.csv", "asn_number,name\n16509,AMAZON\n14061,DIGITALOCEAN\n")
	c := NewClassifier(&Config{DatacenterFile: asnFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "192.0.2.1", "16509")

	assertHeaderVal(t, req, TrafficDatacenterHeader, "true")
	assertHeaderVal(t, req, TrafficTypeHeader, "datacenter")
}

func TestClassifyResidential(t *testing.T) {
	asnFile := writeTempFile(t, "asn.csv", "asn_number,name\n16509,AMAZON\n")
	c := NewClassifier(&Config{DatacenterFile: asnFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "198.51.100.1", "3320")

	assertHeaderVal(t, req, TrafficDatacenterHeader, "false")
	assertHeaderVal(t, req, TrafficTypeHeader, "residential")
}

func TestClassifyVPN(t *testing.T) {
	vpnFile := writeTempFile(t, "vpn.txt", "192.0.2.0/24\n198.51.100.0/24\n")
	c := NewClassifier(&Config{VPNFile: vpnFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "198.51.100.50", "")

	assertHeaderVal(t, req, TrafficVPNHeader, "true")
	assertHeaderVal(t, req, TrafficTypeHeader, "vpn")
}

func TestClassifyVPNNoMatch(t *testing.T) {
	vpnFile := writeTempFile(t, "vpn.txt", "192.0.2.0/24\n")
	c := NewClassifier(&Config{VPNFile: vpnFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "203.0.113.1", "")

	assertHeaderVal(t, req, TrafficVPNHeader, "false")
}

func TestClassifyTor(t *testing.T) {
	torFile := writeTempFile(t, "tor.txt", "# Tor exit nodes\n203.0.113.10\n203.0.113.11\n")
	c := NewClassifier(&Config{TorFile: torFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "203.0.113.10", "")

	assertHeaderVal(t, req, TrafficTorHeader, "true")
	assertHeaderVal(t, req, TrafficTypeHeader, "tor")
}

func TestClassifyAIBot(t *testing.T) {
	c := NewClassifier(&Config{})

	tests := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (compatible; GPTBot/1.0)", "true"},
		{"ClaudeBot/1.0", "true"},
		{"Mozilla/5.0 (compatible; Google-Extended)", "true"},
		{"Mozilla/5.0 (compatible; Bytespider)", "true"},
		{"anthropic-ai", "true"},
		{"PerplexityBot/1.0", "true"},
		{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36", "false"},
		{"curl/7.68.0", "false"},
		{"", "false"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", tt.ua)
		c.Classify(req, "192.0.2.1", "")
		if got := req.Header.Get(TrafficAIBotHeader); got != tt.want {
			t.Errorf("UA=%q: got X-Traffic-AI-Bot=%q, want %q", tt.ua, got, tt.want)
		}
	}
}

func TestClassifyAIBotTrafficType(t *testing.T) {
	c := NewClassifier(&Config{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "ClaudeBot/1.0")
	c.Classify(req, "192.0.2.1", "")

	assertHeaderVal(t, req, TrafficTypeHeader, "ai-crawler")
}

func TestClassifyPriority(t *testing.T) {
	asnFile := writeTempFile(t, "asn.csv", "asn_number,name\n16509,AMAZON\n")
	torFile := writeTempFile(t, "tor.txt", "192.0.2.1\n")
	vpnFile := writeTempFile(t, "vpn.txt", "192.0.2.0/24\n")

	c := NewClassifier(&Config{
		DatacenterFile: asnFile,
		VPNFile:        vpnFile,
		TorFile:        torFile,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "192.0.2.1", "16509")
	assertHeaderVal(t, req, TrafficTypeHeader, "tor")
	assertHeaderVal(t, req, TrafficTorHeader, "true")
	assertHeaderVal(t, req, TrafficVPNHeader, "true")
	assertHeaderVal(t, req, TrafficDatacenterHeader, "true")
}

func TestClassifyMissingFiles(t *testing.T) {
	c := NewClassifier(&Config{
		DatacenterFile: "/nonexistent/asn.csv",
		VPNFile:        "/nonexistent/vpn.txt",
		TorFile:        "/nonexistent/tor.txt",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "192.0.2.1", "")

	assertHeaderVal(t, req, TrafficTypeHeader, "residential")
}

func TestClassifyMissingFilesRetries(t *testing.T) {
	c := NewClassifier(&Config{
		DatacenterFile: "/nonexistent/asn.csv",
	})
	c.mu.RLock()
	refresh := c.lastRefresh
	c.mu.RUnlock()

	expectedRetryAt := time.Now().Add(-time.Duration(c.refreshSeconds)*time.Second + time.Duration(retrySeconds)*time.Second)
	if refresh.After(expectedRetryAt.Add(2 * time.Second)) {
		t.Fatal("lastRefresh should be set for early retry when all loads fail")
	}
}

func TestClassifyEmptyInputs(t *testing.T) {
	c := NewClassifier(&Config{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "", "")

	assertHeaderVal(t, req, TrafficTypeHeader, "residential")
	assertHeaderVal(t, req, TrafficDatacenterHeader, "false")
	assertHeaderVal(t, req, TrafficVPNHeader, "false")
	assertHeaderVal(t, req, TrafficTorHeader, "false")
	assertHeaderVal(t, req, TrafficAIBotHeader, "false")
}

func TestClassifyUnknownASN(t *testing.T) {
	asnFile := writeTempFile(t, "asn.csv", "asn_number,name\n16509,AMAZON\n")
	c := NewClassifier(&Config{DatacenterFile: asnFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "192.0.2.1", Unknown)

	assertHeaderVal(t, req, TrafficDatacenterHeader, "false")
}

func TestClassifyNilReceiver(t *testing.T) {
	var c *Classifier
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "192.0.2.1", "16509")

	if req.Header.Get(TrafficTypeHeader) != "" {
		t.Fatal("nil classifier should not set any headers")
	}
}

func TestClassifyIgnoresSpoofedHeaders(t *testing.T) {
	asnFile := writeTempFile(t, "asn.csv", "asn_number,name\n16509,AMAZON\n")
	torFile := writeTempFile(t, "tor.txt", "203.0.113.99\n")
	c := NewClassifier(&Config{DatacenterFile: asnFile, TorFile: torFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(ASNSystemNumberHeader, "16509")
	req.Header.Set(IPAddressHeader, "203.0.113.99")

	// Classify with trusted values — the spoofed headers should be irrelevant
	c.Classify(req, "198.51.100.1", "3320")

	assertHeaderVal(t, req, TrafficDatacenterHeader, "false")
	assertHeaderVal(t, req, TrafficTorHeader, "false")
	assertHeaderVal(t, req, TrafficTypeHeader, "residential")
}

func TestStripHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(CountryHeader, "Fakeland")
	req.Header.Set(TrafficTypeHeader, "residential")
	req.Header.Set(TrafficDatacenterHeader, "false")
	req.Header.Set(IPAddressHeader, "spoofed")
	req.Header.Set(ASNSystemNumberHeader, "99999")

	StripHeaders(req)

	if req.Header.Get(CountryHeader) != "" {
		t.Fatal("StripHeaders should remove X-GeoIP-Country")
	}
	if req.Header.Get(TrafficTypeHeader) != "" {
		t.Fatal("StripHeaders should remove X-Traffic-Type")
	}
	if req.Header.Get(IPAddressHeader) != "" {
		t.Fatal("StripHeaders should remove X-GeoIP-IPAddress")
	}
	if req.Header.Get(ASNSystemNumberHeader) != "" {
		t.Fatal("StripHeaders should remove X-GeoIP-ASN-System-Number")
	}
}

func TestSpoofedOutputHeadersStripped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TrafficTypeHeader, "residential")
	req.Header.Set(TrafficDatacenterHeader, "false")
	req.Header.Set(TrafficVPNHeader, "false")

	StripHeaders(req)

	if req.Header.Get(TrafficTypeHeader) != "" {
		t.Fatal("client-sent X-Traffic-Type must be stripped")
	}
}

func TestLoadDatacenterASNs(t *testing.T) {
	path := writeTempFile(t, "asn.csv", "asn_number,name,extra\n16509,AMAZON,us\n14061,DIGITALOCEAN,us\n")
	asns, err := loadDatacenterASNs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(asns) != 2 {
		t.Fatalf("expected 2 ASNs, got %d", len(asns))
	}
	if !asns["16509"] || !asns["14061"] {
		t.Fatal("expected 16509 and 14061 in set")
	}
}

func TestLoadVPNNetworks(t *testing.T) {
	path := writeTempFile(t, "vpn.txt", "# comment\n10.0.0.0/8\n192.168.1.1\n\nbadline\n172.16.0.0/12\n")
	nets, skipped, err := loadVPNNetworks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(nets))
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", skipped)
	}
}

func TestLoadTorExits(t *testing.T) {
	path := writeTempFile(t, "tor.txt", "# Tor bulk exit list\n203.0.113.240\n203.0.113.241\n\n")
	exits, err := loadTorExits(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 2 {
		t.Fatalf("expected 2 exits, got %d", len(exits))
	}
}

func TestLoadAIBots(t *testing.T) {
	path := writeTempFile(t, "bots.txt", "# AI bots\nGPTBot\nClaudeBot\n\n")
	bots, err := loadAIBots(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(bots) != 2 {
		t.Fatalf("expected 2 bots, got %d", len(bots))
	}
	if bots[0] != "gptbot" || bots[1] != "claudebot" {
		t.Fatalf("expected pre-lowercased bots, got %v", bots)
	}
}

func TestClassifyWithAIBotFile(t *testing.T) {
	botFile := writeTempFile(t, "bots.txt", "CustomBot\n")
	c := NewClassifier(&Config{AIBotFile: botFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 CustomBot/1.0")
	c.Classify(req, "192.0.2.1", "")

	assertHeaderVal(t, req, TrafficAIBotHeader, "true")
	assertHeaderVal(t, req, TrafficTypeHeader, "ai-crawler")
}

func TestCheckAIBot(t *testing.T) {
	c := NewClassifier(&Config{})
	if c.checkAIBot("") {
		t.Fatal("empty UA should not match")
	}
	if !c.checkAIBot("Mozilla/5.0 (compatible; GPTBot/1.0)") {
		t.Fatal("GPTBot should match")
	}
	if c.checkAIBot("Mozilla/5.0 (X11; Linux x86_64)") {
		t.Fatal("normal browser should not match")
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "true" {
		t.Fatal("expected true")
	}
	if boolStr(false) != "false" {
		t.Fatal("expected false")
	}
}

func TestVPNSingleIP(t *testing.T) {
	vpnFile := writeTempFile(t, "vpn.txt", "192.0.2.1\n")
	c := NewClassifier(&Config{VPNFile: vpnFile})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req, "192.0.2.1", "")
	assertHeaderVal(t, req, TrafficVPNHeader, "true")

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Classify(req2, "192.0.2.2", "")
	assertHeaderVal(t, req2, TrafficVPNHeader, "false")
}

func TestConcurrentClassify(t *testing.T) {
	asnFile := writeTempFile(t, "asn.csv", "asn_number,name\n16509,AMAZON\n")
	vpnFile := writeTempFile(t, "vpn.txt", "192.0.2.0/24\n")
	torFile := writeTempFile(t, "tor.txt", "203.0.113.50\n")
	c := NewClassifier(&Config{
		DatacenterFile: asnFile,
		VPNFile:        vpnFile,
		TorFile:        torFile,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			c.Classify(req, "192.0.2.1", "16509")
			if req.Header.Get(TrafficTypeHeader) == "" {
				t.Error("expected traffic type to be set")
			}
		}()
	}
	wg.Wait()
}

func TestDefaultRefreshSeconds(t *testing.T) {
	c := NewClassifier(&Config{})
	if c.refreshSeconds != defaultRefreshSeconds {
		t.Fatalf("expected default refresh %d, got %d", defaultRefreshSeconds, c.refreshSeconds)
	}
}

func TestConfigNotMutated(t *testing.T) {
	cfg := &Config{}
	NewClassifier(cfg)
	if cfg.RefreshSeconds != 0 {
		t.Fatal("NewClassifier should not mutate the caller's Config")
	}
}

func assertHeaderVal(t *testing.T, req *http.Request, key, expected string) {
	t.Helper()
	if got := req.Header.Get(key); got != expected {
		t.Fatalf("header [%s]: got %q, want %q", key, got, expected)
	}
}
