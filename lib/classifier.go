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
	"strings"
	"sync"
)

var defaultAIBots = []string{
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

// Classifier holds traffic classification data loaded once at startup.
// All fields are immutable after construction — no mutex needed.
type Classifier struct {
	datacenterASNs map[string]bool
	vpnNets        []*net.IPNet
	torExits       map[string]bool
	aiBots         []string
}

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
		torExits:       make(map[string]bool),
		aiBots:         defaultAIBots,
	}
	if err := c.loadData(config); err != nil {
		return nil, err
	}
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

	userAgent := req.Header.Get("User-Agent")

	isDatacenter := c.checkDatacenter(asnNumber)
	isVPN := c.checkVPN(ipStr)
	isTor := c.checkTor(ipStr)
	isAIBot := c.checkAIBot(userAgent)

	trafficType := "residential"
	if isTor {
		trafficType = "tor"
	} else if isVPN {
		trafficType = "vpn"
	} else if isAIBot {
		trafficType = "ai-crawler"
	} else if isDatacenter {
		trafficType = "datacenter"
	}

	req.Header.Set(TrafficTypeHeader, trafficType)
	req.Header.Set(TrafficDatacenterHeader, boolStr(isDatacenter))
	req.Header.Set(TrafficVPNHeader, boolStr(isVPN))
	req.Header.Set(TrafficTorHeader, boolStr(isTor))
	req.Header.Set(TrafficAIBotHeader, boolStr(isAIBot))
}

func (c *Classifier) loadData(config *Config) error {
	if config.DatacenterFile != "" {
		asns, err := loadDatacenterASNs(config.DatacenterFile)
		if err != nil {
			return fmt.Errorf("failed to load datacenter ASNs: %w", err)
		}
		c.datacenterASNs = asns
		log.Printf("[traefik-classifier] Loaded %d datacenter ASNs", len(asns))
	}

	if config.VPNFile != "" {
		nets, skipped, err := loadVPNNetworks(config.VPNFile)
		if err != nil {
			return fmt.Errorf("failed to load VPN networks: %w", err)
		}
		c.vpnNets = nets
		if skipped > 0 {
			log.Printf("[traefik-classifier] Loaded %d VPN networks (%d invalid lines skipped)", len(nets), skipped)
		} else {
			log.Printf("[traefik-classifier] Loaded %d VPN networks", len(nets))
		}
	}

	if config.TorFile != "" {
		exits, err := loadTorExits(config.TorFile)
		if err != nil {
			return fmt.Errorf("failed to load Tor exits: %w", err)
		}
		c.torExits = exits
		log.Printf("[traefik-classifier] Loaded %d Tor exit nodes", len(exits))
	}

	if config.AIBotFile != "" {
		bots, err := loadAIBots(config.AIBotFile)
		if err != nil {
			return fmt.Errorf("failed to load AI bots: %w", err)
		}
		c.aiBots = bots
		log.Printf("[traefik-classifier] Loaded %d AI bot patterns", len(bots))
	}

	return nil
}

func (c *Classifier) checkDatacenter(asn string) bool {
	if asn == "" || asn == Unknown {
		return false
	}
	return c.datacenterASNs[asn]
}

func (c *Classifier) checkVPN(ipStr string) bool {
	if ipStr == "" || ipStr == Unknown {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range c.vpnNets {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (c *Classifier) checkTor(ipStr string) bool {
	return c.torExits[ipStr]
}

func (c *Classifier) checkAIBot(ua string) bool {
	if ua == "" {
		return false
	}
	uaLower := strings.ToLower(ua)
	for _, bot := range c.aiBots {
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

func loadDatacenterASNs(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

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
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var nets []*net.IPNet
	skipped := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "/") {
			line = line + "/32"
		}
		_, cidr, err := net.ParseCIDR(line)
		if err != nil {
			skipped++
			continue
		}
		nets = append(nets, cidr)
	}

	return nets, skipped, nil
}

func loadTorExits(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	exits := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		exits[line] = true
	}

	return exits, nil
}

func loadAIBots(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var bots []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		bots = append(bots, strings.ToLower(line))
	}

	return bots, nil
}
