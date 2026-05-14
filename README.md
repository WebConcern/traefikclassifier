# Traefik Classifier

[![Build](https://github.com/WebConcern/traefikclassifier/actions/workflows/build.yaml/badge.svg)](https://github.com/WebConcern/traefikclassifier/actions/workflows/build.yaml)

Traefik middleware plugin that enriches HTTP requests with geographic and network information from [MaxMind](https://www.maxmind.com) GeoIP2 / GeoLite2 databases. Downstream services receive location and network headers without any application-level GeoIP logic.

Supports [GeoIP2](https://www.maxmind.com/en/geoip2-databases) (commercial) and [GeoLite2](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) (free) databases.

## Features

- **City, Country, and ASN** database support (mix and match)
- **Light mode** reduces header count to codes only (no full names)
- **Multiple IP sources** RemoteAddr, `X-Forwarded-For`, or any custom header
- **ISO-8859-1 encoding** option for legacy backends
- **Zero external Go dependencies** self-contained MMDB reader

## Installation

### From the Traefik Plugin Catalog (recommended)

Add the plugin to your Traefik static configuration:

**traefik.yml**
```yaml
experimental:
  plugins:
    traefikclassifier:
      moduleName: github.com/WebConcern/traefikclassifier
      version: v0.1.0
```

**CLI flags**
```
--experimental.plugins.traefikclassifier.modulename=github.com/WebConcern/traefikclassifier
--experimental.plugins.traefikclassifier.version=v0.1.0
```

Then define middleware via labels, file provider, or a Kubernetes CRD (see sections below).

### Kubernetes (Helm)

Using the [official Traefik Helm chart](https://artifacthub.io/packages/helm/traefik/traefik), add to `values.yaml`:

```yaml
experimental:
  plugins:
    traefikclassifier:
      moduleName: github.com/WebConcern/traefikclassifier
      version: v0.1.0

deployment:
  additionalVolumes:
    - name: geoip2
      emptyDir: {}
  initContainers:
    - name: init-geoip
      image: curlimages/curl
      volumeMounts:
        - name: geoip2
          mountPath: /data/geoip2
      command: ["sh", "-c"]
      args:
        - |
          CITY_DB="/data/geoip2/GeoLite2-City.mmdb"
          ASN_DB="/data/geoip2/GeoLite2-ASN.mmdb"
          [ ! -f "$CITY_DB" ] && curl -LfsS https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb -o "$CITY_DB"
          [ ! -f "$ASN_DB" ] && curl -LfsS https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb -o "$ASN_DB"

additionalVolumeMounts:
  - name: geoip2
    mountPath: /geoip2
```

Create a Traefik Middleware resource:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: traefikclassifier
spec:
  plugin:
    traefikclassifier:
      cityDbPath: "/geoip2/GeoLite2-City.mmdb"
      asnDbPath: "/geoip2/GeoLite2-ASN.mmdb"
```

Apply it to an IngressRoute:

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: my-app
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

### Docker Compose

```yaml
services:
  traefik:
    image: "traefik:v3.2"
    command:
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entryPoints.web.address=:80"
      - "--experimental.plugins.traefikclassifier.modulename=github.com/WebConcern/traefikclassifier"
      - "--experimental.plugins.traefikclassifier.version=v0.1.0"
    ports:
      - "80:80"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
      - geoipupdate_data:/usr/share/GeoIP

  whoami:
    image: "traefik/whoami"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.whoami.rule=Host(`localhost`)"
      - "traefik.http.routers.whoami.entrypoints=web"
      - "traefik.http.middlewares.traefikclassifier.plugin.traefikclassifier.cityDbPath=/usr/share/GeoIP/GeoLite2-City.mmdb"
      - "traefik.http.middlewares.traefikclassifier.plugin.traefikclassifier.asnDbPath=/usr/share/GeoIP/GeoLite2-ASN.mmdb"
      - "traefik.http.routers.whoami.middlewares=traefikclassifier"

  geoipupdate:
    image: ghcr.io/maxmind/geoipupdate
    env_file:
      - ./maxmind.env
    volumes:
      - geoipupdate_data:/usr/share/GeoIP

volumes:
  geoipupdate_data:
```

### Local Plugin (development)

For local development without the plugin catalog:

```yaml
command:
  - "--experimental.localPlugins.traefikclassifier.moduleName=github.com/WebConcern/traefikclassifier"
volumes:
  - "./path/to/traefikclassifier:/plugins-local/src/github.com/WebConcern/traefikclassifier"
```

## Configuration

| Option | Type | Default | Description |
|---|---|---|---|
| `cityDbPath` | string | `""` | Path to a GeoIP2/GeoLite2 City database (.mmdb) |
| `countryDbPath` | string | `""` | Path to a GeoIP2/GeoLite2 Country database (.mmdb) |
| `asnDbPath` | string | `""` | Path to a GeoIP2/GeoLite2 ASN database (.mmdb) |
| `preferXForwardedForHeader` | bool | `false` | Use `X-Forwarded-For` header to determine client IP |
| `ipHeader` | string | `""` | Custom header to read client IP from (overrides RemoteAddr and X-Forwarded-For) |
| `failInError` | bool | `false` | Fail hard if a database cannot be loaded (default: log and continue) |
| `debug` | bool | `false` | Enable debug logging |
| `lightMode` | bool | `false` | Only set code/number headers, omit full-name headers |
| `iso88591` | bool | `false` | Encode header values in ISO-8859-1 instead of UTF-8 |

Provide at least one database path (`cityDbPath`, `countryDbPath`, or `asnDbPath`). You can combine City + ASN or Country + ASN for richer data.

## Output Headers

Headers are set on the **incoming request** so downstream services can read them.

### Geographic (City database)

| Header | Example | Light mode |
|---|---|---|
| `GeoIP-Continent` | `Europe` | omitted |
| `GeoIP-Continent-Code` | `EU` | included |
| `GeoIP-Country` | `Germany` | omitted |
| `GeoIP-Country-Code` | `DE` | included |
| `GeoIP-Region` | `Bavaria` | omitted |
| `GeoIP-Region-Code` | `BY` | included |
| `GeoIP-City` | `Munich` | omitted |
| `GeoIP-Postal-Code` | `80331` | included |
| `GeoIP-Latitude` | `48.1374` | included |
| `GeoIP-Longitude` | `11.5755` | included |
| `GeoIP-Accuracy-Radius` | `50000` | included |
| `GeoIP-Geohash` | `u281yewbzh00` | included |

### Geographic (Country database)

| Header | Example |
|---|---|
| `GeoIP-Country` | `Germany` |
| `GeoIP-Country-Code` | `DE` |

### Network (ASN database)

| Header | Example |
|---|---|
| `GeoIP-ASN-System-Number` | `24940` |
| `GeoIP-ASN-Organization` | `Hetzner Online GmbH` |

### Always set

| Header | Example |
|---|---|
| `GeoIP-IPAddress` | `188.193.88.199` |

Unknown values are returned as `XX`.

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

### Option 2: Community mirror (quick start)

Download directly in an init container:

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

## Attribution

This project builds on work from:

- [traefik-plugins/traefikgeoip2](https://github.com/traefik-plugins/traefikgeoip2)
- [thiagotognoli/traefikgeoip](https://github.com/thiagotognoli/traefikgeoip)
- [Maronato/traefik_geoip](https://github.com/Maronato/traefik_geoip)
- [IncSW/geoip2](https://github.com/IncSW/geoip2) (vendored MMDB reader, MIT license)

## License

[Apache License 2.0](LICENSE)
