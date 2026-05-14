package lib

import (
	"bufio"
	"encoding/csv"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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

const defaultRefreshSeconds = 21600

const retrySeconds = 30

// Classifier holds traffic classification data loaded from external lists.
type Classifier struct {
	mu             sync.RWMutex
	datacenterASNs map[string]bool
	vpnNets        []*net.IPNet
	torExits       map[string]bool
	aiBots         []string
	lastRefresh    time.Time
	refreshSeconds int
	config         *Config
}

// NewClassifier creates a new Classifier and performs the initial data load.
func NewClassifier(config *Config) *Classifier {
	refreshSeconds := config.RefreshSeconds
	if refreshSeconds <= 0 {
		refreshSeconds = defaultRefreshSeconds
	}
	c := &Classifier{
		datacenterASNs: make(map[string]bool),
		torExits:       make(map[string]bool),
		aiBots:         defaultAIBots,
		refreshSeconds: refreshSeconds,
		config:         config,
	}
	c.loadData()
	return c
}

// Classify sets traffic classification headers on the request.
// ipStr and asnNumber must come from trusted middleware lookups, not from request headers.
func (c *Classifier) Classify(req *http.Request, ipStr, asnNumber string) {
	if c == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[traefik-classifier] Recovered from panic: %v", r)
		}
	}()

	c.mu.RLock()
	needsRefresh := time.Since(c.lastRefresh) > time.Duration(c.refreshSeconds)*time.Second
	c.mu.RUnlock()

	if needsRefresh {
		c.loadData()
	}

	userAgent := req.Header.Get("User-Agent")

	c.mu.RLock()
	isDatacenter := c.checkDatacenter(asnNumber)
	isVPN := c.checkVPN(ipStr)
	isTor := c.checkTor(ipStr)
	isAIBot := c.checkAIBot(userAgent)
	c.mu.RUnlock()

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

func (c *Classifier) loadData() {
	var newASNs map[string]bool
	var newVPNs []*net.IPNet
	var newTors map[string]bool
	var newBots []string
	anyLoaded := false

	if c.config.DatacenterFile != "" {
		if asns, err := loadDatacenterASNs(c.config.DatacenterFile); err == nil {
			newASNs = asns
			anyLoaded = true
			log.Printf("[traefik-classifier] Loaded %d datacenter ASNs", len(asns))
		} else {
			log.Printf("[traefik-classifier] Failed to load datacenter ASNs: %v", err)
		}
	}

	if c.config.VPNFile != "" {
		if nets, skipped, err := loadVPNNetworks(c.config.VPNFile); err == nil {
			newVPNs = nets
			anyLoaded = true
			if skipped > 0 {
				log.Printf("[traefik-classifier] Loaded %d VPN networks (%d invalid lines skipped)", len(nets), skipped)
			} else {
				log.Printf("[traefik-classifier] Loaded %d VPN networks", len(nets))
			}
		} else {
			log.Printf("[traefik-classifier] Failed to load VPN networks: %v", err)
		}
	}

	if c.config.TorFile != "" {
		if exits, err := loadTorExits(c.config.TorFile); err == nil {
			newTors = exits
			anyLoaded = true
			log.Printf("[traefik-classifier] Loaded %d Tor exit nodes", len(exits))
		} else {
			log.Printf("[traefik-classifier] Failed to load Tor exits: %v", err)
		}
	}

	if c.config.AIBotFile != "" {
		if bots, err := loadAIBots(c.config.AIBotFile); err == nil {
			newBots = bots
			anyLoaded = true
			log.Printf("[traefik-classifier] Loaded %d AI bot patterns", len(bots))
		} else {
			log.Printf("[traefik-classifier] Failed to load AI bots: %v", err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastRefresh) <= time.Duration(c.refreshSeconds)*time.Second {
		return
	}

	if newASNs != nil {
		c.datacenterASNs = newASNs
	}
	if newVPNs != nil {
		c.vpnNets = newVPNs
	}
	if newTors != nil {
		c.torExits = newTors
	}
	if newBots != nil {
		c.aiBots = newBots
	}

	hasConfiguredFiles := c.config.DatacenterFile != "" || c.config.VPNFile != "" ||
		c.config.TorFile != "" || c.config.AIBotFile != ""

	if !hasConfiguredFiles || anyLoaded {
		c.lastRefresh = time.Now()
	} else {
		// All configured files failed — retry sooner instead of waiting the full refresh interval
		c.lastRefresh = time.Now().Add(-time.Duration(c.refreshSeconds)*time.Second + time.Duration(retrySeconds)*time.Second)
		log.Printf("[traefik-classifier] All file loads failed, retrying in %ds", retrySeconds)
	}
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
