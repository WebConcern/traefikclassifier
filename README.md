# Traefik Classifier

[![Build](https://github.com/WebConcern/traefikclassifier/actions/workflows/build.yaml/badge.svg)](https://github.com/WebConcern/traefikclassifier/actions/workflows/build.yaml)

[Traefik](https://doc.traefik.io/traefik/) middleware plugin that enriches HTTP requests with geographic location data from [MaxMind](https://www.maxmind.com) GeoIP2/GeoLite2 databases and classifies traffic by type (datacenter, VPN, Tor, AI crawler).

All results are passed downstream via HTTP request headers. Downstream services can read these headers without any GeoIP logic of their own.

Supports both [GeoIP2](https://www.maxmind.com/en/geoip2-databases) (commercial) and [GeoLite2](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) (free) databases.

## Features

- **GeoIP lookups** -- City, Country, and ASN data from MaxMind databases
- **Datacenter detection** -- ASN-based, using [bad-asn-list](https://github.com/brianhama/bad-asn-list)
- **VPN detection** -- CIDR-based, using [X4BNet/lists_vpn](https://github.com/X4BNet/lists_vpn)
- **Tor exit node detection** -- IP-based, using the [Tor Project bulk exit list](https://check.torproject.org/torbulkexitlist)
- **AI crawler detection** -- User-Agent based (GPTBot, ClaudeBot, Google-Extended, Bytespider, and more)
- **Header stripping** -- Inbound `X-GeoIP-*` and `X-Traffic-*` headers are removed before processing, preventing clients from spoofing
- **Singleton architecture** -- All data (GeoIP databases, classification lists) is loaded once and shared across all routes
- **Hot reload** -- Data files are checked every `refreshSeconds` and reloaded when they change, without restarting Traefik
- **Fails open** -- A missing, broken or empty data file is logged and never takes routes down; previously loaded data stays in use
- **Light mode** -- Reduces header count to essential fields only
- **Zero external Go dependencies** -- Self-contained MMDB reader (vendored from [IncSW/geoip2](https://github.com/IncSW/geoip2))

## Output Headers

### GeoIP Headers (City database)

| Header | Example | Light mode |
|--------|---------|------------|
| `X-GeoIP-Country` | `Germany` | -- |
| `X-GeoIP-Country-Code` | `DE` | included |
| `X-GeoIP-Region` | `Bavaria` | -- |
| `X-GeoIP-Region-Code` | `BY` | included |
| `X-GeoIP-City` | `Munich` | included |
| `X-GeoIP-Postal-Code` | `80331` | -- |
| `X-GeoIP-Latitude` | `48.1374` | included |
| `X-GeoIP-Longitude` | `11.5755` | included |
| `X-GeoIP-Accuracy-Radius` | `50000` | included |
| `X-GeoIP-Geohash` | `u281yewbzh00` | -- |

### GeoIP Headers (Country database)

| Header | Example |
|--------|---------|
| `X-GeoIP-Country` | `Germany` |
| `X-GeoIP-Country-Code` | `DE` |

### GeoIP Headers (ASN database)

| Header | Example |
|--------|---------|
| `X-GeoIP-ASN-System-Number` | `24940` |
| `X-GeoIP-ASN-Organization` | `Hetzner Online GmbH` |

### Other Headers (always set)

| Header | Example |
|--------|---------|
| `X-GeoIP-IPAddress` | `188.193.88.199` |

Unknown values are returned as `XX`.

### Traffic Classification Headers

| Header | Values | Description |
|--------|--------|-------------|
| `X-Traffic-Type` | `residential`, `datacenter`, `vpn`, `tor`, `ai-crawler` | Overall classification |
| `X-Traffic-Datacenter` | `true` / `false` | IP belongs to a known datacenter ASN |
| `X-Traffic-VPN` | `true` / `false` | IP belongs to a known VPN CIDR range |
| `X-Traffic-Tor` | `true` / `false` | IP is a known Tor exit node |
| `X-Traffic-AI-Bot` | `true` / `false` | User-Agent matches a known AI crawler |

`X-Traffic-Type` priority: **tor > vpn > ai-crawler > datacenter > residential**.

The individual flags are independent -- a single request can have `X-Traffic-Datacenter: true` **and** `X-Traffic-Tor: true` at the same time, but `X-Traffic-Type` will be `tor` (highest priority wins).

Traffic classification headers are always set, even without classification data files configured. Without data files, all traffic is classified as `residential` and the built-in AI bot list is still active.

### Built-in AI Crawlers

These User-Agent substrings are detected out of the box (case-insensitive):

GPTBot, ChatGPT-User, OAI-SearchBot, ClaudeBot, Claude-Web, Google-Extended, Bytespider, CCBot, FacebookBot, anthropic-ai, PerplexityBot, Cohere-ai, meta-externalagent

You can replace this list with a custom file via the `aiBotFile` option (one substring per line).

## Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `cityDbPath` | string | `""` | Path to a GeoIP2/GeoLite2 City database (.mmdb) |
| `countryDbPath` | string | `""` | Path to a GeoIP2/GeoLite2 Country database (.mmdb) |
| `asnDbPath` | string | `""` | Path to a GeoIP2/GeoLite2 ASN database (.mmdb) |
| `preferXForwardedForHeader` | bool | `false` | Use the first `X-Forwarded-For` entry as the client IP. That entry is client-controlled, so only enable this when a trusted proxy in front of Traefik overwrites the header |
| `ipHeader` | string | `""` | Custom header to read client IP from (overrides RemoteAddr and X-Forwarded-For) |
| `failInError` | bool | `false` | Fail the middleware (and so its routers) if a GeoIP database cannot be loaded (default: log and continue without GeoIP) |
| `debug` | bool | `false` | Enable debug logging for lookup failures |
| `lightMode` | bool | `false` | Only set code/coordinate headers, omit full names and geohash |
| `iso88591` | bool | `false` | Encode header values in ISO-8859-1 instead of UTF-8 |
| `datacenterFile` | string | `""` | Path to CSV file with datacenter ASNs (first column = ASN number) |
| `vpnFile` | string | `""` | Path to text file with VPN CIDR ranges (one per line) |
| `torFile` | string | `""` | Path to text file with Tor exit node IPs (one per line) |
| `aiBotFile` | string | `""` | Path to text file with AI bot UA substrings (one per line, replaces built-in list) |
| `refreshSeconds` | int | `3600` | How often to check data files and GeoIP databases for changes; `0` disables reloading |

Provide at least one database path (`cityDbPath`, `countryDbPath`, or `asnDbPath`). You can combine City + ASN or Country + ASN for richer data.

## Installation

### Traefik Plugin Catalog

Add the plugin to your Traefik static configuration:

```yaml
# traefik.yml
experimental:
  plugins:
    traefikclassifier:
      moduleName: github.com/WebConcern/traefikclassifier
      version: v0.2.0
```

Or via CLI flags:

```
--experimental.plugins.traefikclassifier.modulename=github.com/WebConcern/traefikclassifier
--experimental.plugins.traefikclassifier.version=v0.2.0
```

Then define middleware via labels, file provider, or a Kubernetes CRD (see sections below).

### Local Plugin (development)

For local development without the plugin catalog:

```yaml
command:
  - "--experimental.localPlugins.traefikclassifier.moduleName=github.com/WebConcern/traefikclassifier"
volumes:
  - "./path/to/traefikclassifier:/plugins-local/src/github.com/WebConcern/traefikclassifier"
```

## Docker Compose Example

Self-contained example using local plugin mode with bundled test databases (no MaxMind license needed):

```yaml
# docker-compose.yaml
services:
  traefik:
    image: "traefik:v3.2"
    command:
      - "--log.level=DEBUG"
      - "--api.insecure=true"
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entryPoints.web.address=:80"
      - "--experimental.localPlugins.traefikclassifier.moduleName=github.com/WebConcern/traefikclassifier"
    ports:
      - "80:80"
      - "8080:8080"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
      - ".:/plugins-local/src/github.com/WebConcern/traefikclassifier"

  whoami:
    image: "traefik/whoami"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.whoami.rule=Host(`localhost`)"
      - "traefik.http.routers.whoami.entrypoints=web"
      - "traefik.http.middlewares.traefikclassifier.plugin.traefikclassifier.cityDbPath=/plugins-local/src/github.com/WebConcern/traefikclassifier/data/mmdb/GeoLite2-City.mmdb"
      - "traefik.http.middlewares.traefikclassifier.plugin.traefikclassifier.asnDbPath=/plugins-local/src/github.com/WebConcern/traefikclassifier/data/mmdb/GeoLite2-ASN.mmdb"
      - "traefik.http.routers.whoami.middlewares=traefikclassifier"
```

```sh
docker compose up -d
curl -s http://localhost
```

Output (whoami echoes all request headers):

```
X-Geoip-Accuracy-Radius: XX
X-Geoip-Asn-Organization: XX
X-Geoip-Asn-System-Number: XX
X-Geoip-City: XX
X-Geoip-Country: XX
X-Geoip-Country-Code: XX
X-Geoip-Geohash: XX
X-Geoip-Ipaddress: 172.24.0.1
X-Geoip-Latitude: XX
X-Geoip-Longitude: XX
X-Geoip-Postal-Code: XX
X-Geoip-Region: XX
X-Geoip-Region-Code: XX
X-Traffic-Ai-Bot: false
X-Traffic-Datacenter: false
X-Traffic-Tor: false
X-Traffic-Type: residential
X-Traffic-Vpn: false
```

GeoIP fields show `XX` because the Docker-internal IP is not in the database. With a real client IP you would see actual location data.

Test AI bot detection:

```sh
curl -s -H "User-Agent: GPTBot/1.0" http://localhost | grep X-Traffic
```

```
X-Traffic-Ai-Bot: true
X-Traffic-Datacenter: false
X-Traffic-Tor: false
X-Traffic-Type: ai-crawler
X-Traffic-Vpn: false
```

## Kubernetes (Helm)

Using the [official Traefik Helm chart](https://artifacthub.io/packages/helm/traefik/traefik). The plugin reads its data from files, so the setup has three parts:

1. **A shared volume** at `/data` that every Traefik pod mounts. It must be `ReadWriteMany` (for example Longhorn, NFS or CephFS) so one CronJob can update the files for all pods.
2. **An init container** that seeds the volume, so a new pod never starts without data.
3. **A CronJob** that keeps the files current. The plugin checks them every `refreshSeconds` and reloads changed files without a restart.

### values.yaml

```yaml
experimental:
  plugins:
    traefikclassifier:
      moduleName: github.com/WebConcern/traefikclassifier
      version: v0.2.0

persistence:
  enabled: true
  name: data
  accessMode: ReadWriteMany
  size: 256Mi
  storageClass: longhorn # any storage class that supports ReadWriteMany
  path: /data

deployment:
  initContainers:
    - name: init-traefik-data
      image: curlimages/curl
      command: ["sh", "-c"]
      args:
        - |
          # fetch URL DEST MAX_AGE_MINUTES
          # All pods share this volume: skip files newer than MAX_AGE_MINUTES, download
          # to a temp file and rename it into place so readers never see a partial file,
          # and keep the existing file if a download fails. A failed download never
          # blocks Traefik from starting; the plugin runs on the data it already has.
          fetch() {
            url="$1"; dest="$2"; max_age="$3"
            if [ -n "$(find "$dest" -mmin -"$max_age" 2>/dev/null)" ]; then
              echo "Up to date: $dest"
              return 0
            fi
            tmp="$dest.tmp.$$"
            if curl -LfsS --retry 3 --max-time 300 "$url" -o "$tmp" && [ -s "$tmp" ]; then
              mv -f "$tmp" "$dest"
              echo "Downloaded: $dest"
            else
              rm -f "$tmp"
              echo "WARNING: download of $url failed, keeping existing $dest" >&2
            fi
          }

          mkdir -p /data/geoip2 /data/traefik-classifier

          fetch https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb /data/geoip2/GeoLite2-City.mmdb 10080
          fetch https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb /data/geoip2/GeoLite2-ASN.mmdb 10080

          fetch https://raw.githubusercontent.com/brianhama/bad-asn-list/master/bad-asn-list.csv /data/traefik-classifier/bad-asn-list.csv 360
          fetch https://raw.githubusercontent.com/X4BNet/lists_vpn/main/output/vpn/ipv4.txt /data/traefik-classifier/vpn-ipv4.txt 360
          fetch https://check.torproject.org/torbulkexitlist /data/traefik-classifier/tor-exits.txt 360
      volumeMounts:
        - name: data
          mountPath: /data
```

The chart names the PVC after the release (`traefik` by default); the CronJob below mounts it by that name.

### Middleware CRD

GeoIP only:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: traefikclassifier
  namespace: traefik
spec:
  plugin:
    traefikclassifier:
      cityDbPath: "/data/geoip2/GeoLite2-City.mmdb"
      asnDbPath: "/data/geoip2/GeoLite2-ASN.mmdb"
```

GeoIP + traffic classification:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: traefikclassifier
  namespace: traefik
spec:
  plugin:
    traefikclassifier:
      cityDbPath: "/data/geoip2/GeoLite2-City.mmdb"
      asnDbPath: "/data/geoip2/GeoLite2-ASN.mmdb"
      datacenterFile: "/data/traefik-classifier/bad-asn-list.csv"
      vpnFile: "/data/traefik-classifier/vpn-ipv4.txt"
      torFile: "/data/traefik-classifier/tor-exits.txt"
      refreshSeconds: 900 # check for updated files every 15 minutes
```

### IngressRoute

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: my-app
  namespace: traefik
spec:
  entryPoints:
    - web
  routes:
    - match: Host(`app.example.com`)
      kind: Rule
      middlewares:
        - name: traefikclassifier
      services:
        - name: my-app
          port: 80
```

### Keeping data current

All data files (GeoIP databases and classification lists) are loaded at startup and checked for changes every `refreshSeconds` (default: one hour). A changed file is reloaded in place, without restarting Traefik. A replacement that fails to load, or a list that is suddenly empty (usually a failed download), is rejected and the previous data stays in use. A GeoIP database that is missing at startup is not picked up later; restart the pod once it exists.

The reload interval is taken from the first middleware instance Traefik creates, since all instances share the same data.

A CronJob that refreshes the classification lists hourly (the Tor exit list changes throughout the day). It writes to a temp file and renames it into place, and fails the job when a download fails so it shows up in monitoring. Update the GeoIP databases the same way on a weekly schedule, or with [geoipupdate](#option-1-maxmind-with-automatic-updates-recommended-for-production).

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: traefik-classifier-updater
  namespace: traefik
spec:
  schedule: "23 * * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      backoffLimit: 2
      ttlSecondsAfterFinished: 300
      template:
        spec:
          containers:
            - name: downloader
              image: curlimages/curl
              command: ["sh", "-c"]
              args:
                - |
                  failed=0
                  fetch() {
                    url="$1"; dest="$2"
                    tmp="$dest.tmp.$$"
                    if curl -LfsS --retry 3 --max-time 300 "$url" -o "$tmp" && [ -s "$tmp" ]; then
                      mv -f "$tmp" "$dest"
                      echo "Updated: $dest"
                    else
                      rm -f "$tmp"
                      echo "ERROR: download of $url failed, keeping existing $dest" >&2
                      failed=1
                    fi
                  }

                  mkdir -p /data/traefik-classifier
                  fetch https://raw.githubusercontent.com/brianhama/bad-asn-list/master/bad-asn-list.csv /data/traefik-classifier/bad-asn-list.csv
                  fetch https://raw.githubusercontent.com/X4BNet/lists_vpn/main/output/vpn/ipv4.txt /data/traefik-classifier/vpn-ipv4.txt
                  fetch https://check.torproject.org/torbulkexitlist /data/traefik-classifier/tor-exits.txt
                  exit $failed
              volumeMounts:
                - name: data
                  mountPath: /data
          securityContext: # match the Traefik pods so both can replace the files
            fsGroup: 65532
            runAsUser: 65532
            runAsGroup: 65532
            runAsNonRoot: true
          restartPolicy: OnFailure
          volumes:
            - name: data
              persistentVolumeClaim:
                claimName: traefik
```

## Classification Data Sources

| Detection | Source | Format |
|-----------|--------|--------|
| Datacenter ASNs | [bad-asn-list](https://github.com/brianhama/bad-asn-list) | CSV (first column = ASN number) |
| VPN ranges | [X4BNet/lists_vpn](https://github.com/X4BNet/lists_vpn) | One CIDR per line (e.g. `10.0.0.0/8`) |
| Tor exit nodes | [torproject.org](https://check.torproject.org/torbulkexitlist) | One IP per line |
| AI crawlers | Built-in list, or custom file via `aiBotFile` | One User-Agent substring per line |

## GeoIP Database Setup

You need MaxMind `.mmdb` database files. Two options:

### Option 1: MaxMind with automatic updates (recommended for production)

1. Create a free account at [MaxMind](https://www.maxmind.com/en/geolite2/signup)
2. Generate a license key in your account settings
3. Use the [geoipupdate](https://github.com/maxmind/geoipupdate) container:

```env
# maxmind.env
GEOIPUPDATE_ACCOUNT_ID=YOUR_ACCOUNT_ID
GEOIPUPDATE_LICENSE_KEY=YOUR_LICENSE_KEY
GEOIPUPDATE_EDITION_IDS=GeoLite2-ASN GeoLite2-City GeoLite2-Country
GEOIPUPDATE_FREQUENCY=72
```

### Option 2: Community mirror (quick start, no account needed)

Download directly in an init container or script:

```sh
curl -LfsS https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb -o /path/to/GeoLite2-City.mmdb
curl -LfsS https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb -o /path/to/GeoLite2-ASN.mmdb
```

## Development

### Prerequisites

```sh
brew install go golangci-lint just
go install github.com/traefik/yaegi/cmd/yaegi@latest
```

### Run tests

```sh
just test
```

This runs formatting, linting, Go tests, and Yaegi interpreter tests.

## Acknowledgements

This project builds on work from:

- [traefik-plugins/traefikgeoip2](https://github.com/traefik-plugins/traefikgeoip2)
- [thiagotognoli/traefikgeoip](https://github.com/thiagotognoli/traefikgeoip)
- [Maronato/traefik_geoip](https://github.com/Maronato/traefik_geoip)
- [IncSW/geoip2](https://github.com/IncSW/geoip2) (vendored MMDB reader, MIT license)

Classification data provided by:

- [MaxMind](https://www.maxmind.com) -- GeoLite2 and GeoIP2 databases
- [P3TERX/GeoLite.mmdb](https://github.com/P3TERX/GeoLite.mmdb) -- Community mirror of GeoLite2 databases
- [brianhama/bad-asn-list](https://github.com/brianhama/bad-asn-list) -- Datacenter/hosting ASN list
- [X4BNet/lists_vpn](https://github.com/X4BNet/lists_vpn) -- VPN provider IP ranges
- [The Tor Project](https://www.torproject.org) -- Tor exit node list
- [mmcloughlin/geohash](https://github.com/mmcloughlin/geohash) -- Geohash encoding (MIT license)

## License

[Apache License 2.0](LICENSE)
