package lib

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// defaultAIBots returns the built-in AI crawler User-Agent substrings (lowercase).
func defaultAIBots() []string {
	return []string{
		"gptbot",
		"chatgpt-user",
		"oai-searchbot",
		"claudebot",
		"claude-web",
		"google-extended",
		"bytespider",
		"ccbot",
		"facebookbot",
		"anthropic-ai",
		"perplexitybot",
		"cohere-ai",
		"meta-externalagent",
	}
}

// Classifier holds traffic classification data. Each list is loaded at startup and
// replaced wholesale when its file changes (see RefreshFiles); a loaded list is never
// mutated, so readers only need the lock to take a snapshot.
type Classifier struct {
	mu             sync.RWMutex
	datacenterASNs map[string]bool
	vpnNets        *ipSet
	torExits       map[string]bool
	aiBots         []string
}

// Traefik calls New once per router using the middleware; all of them share one Classifier.
//
//nolint:gochecknoglobals // process-wide singleton shared by all middleware instances
var (
	classifierMu       sync.Mutex
	classifierInstance *Classifier
)

// NewClassifier returns the shared Classifier singleton, creating it on the first call.
func NewClassifier(config *Config) (*Classifier, error) {
	classifierMu.Lock()
	defer classifierMu.Unlock()

	if classifierInstance != nil {
		return classifierInstance, nil
	}

	c := &Classifier{
		datacenterASNs: make(map[string]bool),
		vpnNets:        newIPSet(nil),
		torExits:       make(map[string]bool),
		aiBots:         defaultAIBots(),
	}
	c.loadData(config)
	classifierInstance = c
	return classifierInstance, nil
}

// ResetClassifier clears the singleton so the next NewClassifier call creates a fresh instance.
func ResetClassifier() {
	classifierMu.Lock()
	defer classifierMu.Unlock()
	classifierInstance = nil
}

// Classify sets traffic classification headers on the request.
// ipStr and asnNumber must come from trusted middleware lookups, not from request headers.
func (c *Classifier) Classify(req *http.Request, ipStr, asnNumber string) {
	if c == nil {
		return
	}

	c.mu.RLock()
	datacenterASNs, vpnNets, torExits, aiBots := c.datacenterASNs, c.vpnNets, c.torExits, c.aiBots
	c.mu.RUnlock()

	ip := net.ParseIP(ipStr)
	isDatacenter := checkDatacenter(datacenterASNs, asnNumber)
	isVPN := ip != nil && vpnNets.contains(ip)
	isTor := ip != nil && torExits[ip.String()]
	isAIBot := checkAIBot(aiBots, req.Header.Get("User-Agent"))

	trafficType := "residential"
	switch {
	case isTor:
		trafficType = "tor"
	case isVPN:
		trafficType = "vpn"
	case isAIBot:
		trafficType = "ai-crawler"
	case isDatacenter:
		trafficType = "datacenter"
	}

	req.Header.Set(TrafficTypeHeader, trafficType)
	req.Header.Set(TrafficDatacenterHeader, boolStr(isDatacenter))
	req.Header.Set(TrafficVPNHeader, boolStr(isVPN))
	req.Header.Set(TrafficTorHeader, boolStr(isTor))
	req.Header.Set(TrafficAIBotHeader, boolStr(isAIBot))
}

// loadData loads every configured list and watches it for changes. A list that
// cannot be loaded is logged and left empty: classification degrades instead of
// failing the middleware, which would take down every router using it.
func (c *Classifier) loadData(config *Config) {
	c.watch("datacenter ASNs", config.DatacenterFile, func(path string) (int, error) {
		asns, err := loadDatacenterASNs(path)
		if err != nil {
			return 0, err
		}
		return c.swap(len(asns), len(c.datacenterASNs), func() { c.datacenterASNs = asns })
	})

	c.watch("VPN networks", config.VPNFile, func(path string) (int, error) {
		nets, skipped, err := loadVPNNetworks(path)
		if err != nil {
			return 0, err
		}
		if skipped > 0 {
			log.Printf("[traefik-classifier] Skipped %d invalid lines in %s", skipped, path)
		}
		set := newIPSet(nets)
		return c.swap(len(nets), c.vpnNets.size, func() { c.vpnNets = set })
	})

	c.watch("Tor exit nodes", config.TorFile, func(path string) (int, error) {
		exits, err := loadTorExits(path)
		if err != nil {
			return 0, err
		}
		return c.swap(len(exits), len(c.torExits), func() { c.torExits = exits })
	})

	// A configured bot file replaces the built-in list, also when it is missing or empty:
	// an empty file is how AI bot detection is turned off.
	if config.AIBotFile != "" {
		c.aiBots = nil
	}
	c.watch("AI bot patterns", config.AIBotFile, func(path string) (int, error) {
		bots, err := loadAIBots(path)
		if err != nil {
			return 0, err
		}
		return c.swap(len(bots), len(c.aiBots), func() { c.aiBots = bots })
	})
}

// watch loads a list from path, if configured, and registers it for reloading.
// load returns the number of entries it swapped in.
func (c *Classifier) watch(what, path string, load func(path string) (int, error)) {
	if path == "" {
		return
	}
	stamp := statFile(path)
	if n, err := load(path); err != nil {
		log.Printf("[traefik-classifier] ERROR: failed to load %s from %s, continuing without them: %v", what, path, err)
	} else {
		log.Printf("[traefik-classifier] Loaded %d %s", n, what)
	}
	watchFile(what, path, stamp, func(path string) error {
		_, err := load(path)
		return err
	})
}

// swap replaces a list under the write lock. It refuses to replace a non-empty list
// with an empty one, which is almost always a failed or truncated download.
func (c *Classifier) swap(newLen, oldLen int, replace func()) (int, error) {
	if newLen == 0 && oldLen > 0 {
		return 0, fmt.Errorf("refusing to replace %d entries with an empty list", oldLen)
	}
	c.mu.Lock()
	replace()
	c.mu.Unlock()
	return newLen, nil
}

func checkDatacenter(asns map[string]bool, asn string) bool {
	if asn == "" || asn == Unknown {
		return false
	}
	return asns[asn]
}

func checkAIBot(bots []string, ua string) bool {
	if ua == "" {
		return false
	}
	uaLower := strings.ToLower(ua)
	for _, bot := range bots {
		if strings.Contains(uaLower, bot) {
			return true
		}
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// openDataFile opens an operator-configured data file for reading.
func openDataFile(path string) (*os.File, error) {
	return os.Open(filepath.Clean(path))
}

// readDataLines calls fn for every line of a data file, skipping blank lines and # comments.
func readDataLines(path string, fn func(line string)) error {
	f, err := openDataFile(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only: a close error cannot lose data

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fn(line)
	}
	return scanner.Err()
}

func loadDatacenterASNs(path string) (map[string]bool, error) {
	f, err := openDataFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only: a close error cannot lose data

	reader := csv.NewReader(f)
	asns := make(map[string]bool)

	if _, err := reader.Read(); err != nil {
		return nil, err
	}

	skipped := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			skipped++
			continue
		}
		if len(record) > 0 {
			asn := strings.TrimSpace(record[0])
			if asn != "" {
				asns[asn] = true
			}
		}
	}

	if skipped > 0 {
		log.Printf("[traefik-classifier] Skipped %d malformed rows in datacenter ASN file", skipped)
	}
	return asns, nil
}

func loadVPNNetworks(path string) ([]*net.IPNet, int, error) {
	var nets []*net.IPNet
	skipped := 0
	err := readDataLines(path, func(line string) {
		if !strings.Contains(line, "/") {
			// A bare address is a single host: /32 for IPv4, /128 for IPv6.
			ip := net.ParseIP(line)
			if ip == nil {
				skipped++
				return
			}
			if ip.To4() != nil {
				line += "/32"
			} else {
				line += "/128"
			}
		}
		_, cidr, err := net.ParseCIDR(line)
		if err != nil {
			skipped++
			return
		}
		nets = append(nets, cidr)
	})
	if err != nil {
		return nil, 0, err
	}
	return nets, skipped, nil
}

func loadTorExits(path string) (map[string]bool, error) {
	exits := make(map[string]bool)
	err := readDataLines(path, func(line string) {
		// Store the canonical form so lookups match however the address is written.
		if ip := net.ParseIP(line); ip != nil {
			exits[ip.String()] = true
		}
	})
	if err != nil {
		return nil, err
	}
	return exits, nil
}

func loadAIBots(path string) ([]string, error) {
	var bots []string
	err := readDataLines(path, func(line string) {
		bots = append(bots, strings.ToLower(line))
	})
	if err != nil {
		return nil, err
	}
	return bots, nil
}
