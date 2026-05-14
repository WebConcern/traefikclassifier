# Test Data

This directory contains **hand-crafted test fixtures** used by the plugin's test suite.

The `.json` files contain a small number of manually defined IP records. The `.mmdb` files
in `mmdb/` are generated from these JSON files using the converter tool (`main.go`).

These are **not** redistributed MaxMind GeoLite2 databases. They use the same MMDB format
and naming conventions for compatibility with the GeoIP2 reader, but the data is synthetic.

## Converter tool

`main.go` is a build-time utility forked from
[oladush/json-to-mmdb](https://github.com/oladush/json-to-mmdb). The upstream repository
has no license. This file is included for development convenience only and is **not** part
of the distributed plugin. It has its own `go.mod` and is excluded from plugin builds.

To regenerate the test MMDB files:

```sh
cd data
go run main.go -i GeoLite2-City.json -o mmdb/GeoLite2-City.mmdb -t GeoLite2-City
go run main.go -i GeoLite2-ASN.json -o mmdb/GeoLite2-ASN.mmdb -t GeoLite2-ASN
go run main.go -i GeoLite2-Country.json -o mmdb/GeoLite2-Country.mmdb -t GeoLite2-Country
```
