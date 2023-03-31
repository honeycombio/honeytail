# Honeytail Changelog

## 1.8.2

### Maintenance

- Bump github.com/stretchr/testify from 1.8.0 to 1.8.1 (#280) | dependabot
- Bump github.com/klauspost/compress from 1.15.9 to 1.15.12 (#279) | dependabot
- Bump github.com/honeycombio/libhoney-go from 1.16.0 to 1.18.0 (#278) | dependabot
- maint: delete workflows for old board (#277) | [@vreynolds](https://github.com/vreynolds)
- maint: add release file (#275) | [@vreynolds](https://github.com/vreynolds)
- maint: add new project workflow (#274) | [@vreynolds](https://github.com/vreynolds)
- doc: add mysql example (#270) | [@vreynolds](https://github.com/vreynolds)
- doc: add nginx example (#269) | [@vreynolds](https://github.com/vreynolds)
- doc: add haproxy example (#268) | [@vreynolds](https://github.com/vreynolds)

## 1.8.1

### Fixes
- Correct Dockerfile to use Go 1.18 properly (#266) | [@kentquirk](https://github.com/kentquirk)

## 1.8.0

### Enhancements:

- Support YAML configs (#262) | [@kentquirk](https://github.com/kentquirk)

### Maintenance:

- Bump github.com/sirupsen/logrus from 1.8.1 to 1.9.0 (#258) | dependabot
- Bump github.com/honeycombio/libhoney-go from 1.15.8 to 1.16.0 (#259) | dependabot
- Bump github.com/klauspost/compress from 1.15.8 to 1.15.9 (#260) | dependabot

### Fixes:

- Fix consistency bugs in timestamp processing (#263) | [@kentquirk](https://github.com/kentquirk)
- Remove dependency on (sunsetted) mongodbtools (#264) | [@kentquirk](https://github.com/kentquirk)

## 1.7.1

### Maintenance

- Bump github.com/klauspost/compress from 1.15.5 to 1.15.8 (#255) | dependabot
- fixes openSSL CVE

## 1.7.0

### Fixes

- fix(postgres): report query duration as duration_ms (#251) | [pckilgore](https://github.com/pckilgore)

### Maintenance

- Bump github.com/klauspost/compress from 1.15.1 to 1.15.5 (#249)

## 1.6.2

### Maintenance

- [maint] update circle to cimg/go:1.18, update alpine to 3.13 (#246) | [@JamieDanielson](https://github.com/JamieDanielson)
  - fixes openSSL CVE
- Bump github.com/stretchr/testify from 1.7.0 to 1.7.1 (#244) | [dependabot](https://github.com/dependabot)
- Bump github.com/klauspost/compress from 1.13.6 to 1.15.1 (#245) | [dependabot](https://github.com/dependabot)

## 1.6.1

### Maintenance

- Update go and libhoney (#236) | [@MikeGoldsmith](https://github.com/MikeGoldsmith)
- gh: add re-triage workflow (#235) | [@vreynolds](https://github.com/vreynolds)
- docs: add example (#232) | [@JamieDanielson](https://github.com/jamiedanielson)
- Update dependabot to monthly (#233) | [@vreynolds](https://github.com/vreynolds)
- docs: add config usage to readme (#231) | [@vreynolds](https://github.com/vreynolds)
- Update install docs for modern go versions (#230) | [@vreynolds](https://github.com/vreynolds)

## 1.6.0

Improvements:

- Parse trace data from SQL comments (#226) | [@endor](https://github.com/endor)

Maintenance:

- bump libhoney-go to v1.15.6 (#229)
- empower apply-labels action to apply labels (#227)
- Bump github.com/honeycombio/libhoney-go from 1.15.4 to 1.15.5 (#225)
- Change maintenance badge to maintained (#223)
- Adds Stalebot (#224)
- Bump github.com/klauspost/compress from 1.13.4 to 1.13.6 (#222)
- Bump github.com/honeycombio/libhoney-go from 1.15.3 to 1.15.4 (#212)
- Bump github.com/klauspost/compress from 1.13.1 to 1.13.4 (#217)
- Add issue and PR templates (#218)
- Add OSS lifecycle badge (#216)
- Add community health files (#215)
- Bump github.com/klauspost/compress from 1.12.2 to 1.13.1 (#208)
- Updates GitHub Action Workflows (#211)
- Updates Dependabot Config (#210)
- Switches CODEOWNERS to telemetry-team (#209)
- Bump github.com/honeycombio/libhoney-go from 1.15.2 to 1.15.3 (#206)
- arm[32] support, for raspberry pis. (raspberries pi?) (#205)
- Bump github.com/klauspost/compress from 1.11.13 to 1.12.2 (#198)

## 1.5.0

Improvements:

- Now building for darwin/arm64 (M1)!

## 1.4.1

Fixes:

- Generate statefile hash based on tailed file location (#193)

## 1.4.0

Improvements:

- Add tail option to generate hash per state file when tailing multiple files with same name (#191)
- Include note about quoted table/column names in PG (#186)

Maintenance:

- Teach Dependabot to use our maintenance labels (#180 & #181)
- Bump github.com/go-sql-driver/mysql from 1.5.0 to 1.6.0 (#188)
- Bump github.com/klauspost/compress from 1.11.12 to 1.11.13 (#187)
- Bump github.com/stretchr/testify from 1.5.1 to 1.7.0 (#169)
- Bump github.com/honeycombio/libhoney-go from 1.12.4 to 1.15.2 (#171)
- Bump github.com/klauspost/compress from 1.10.3 to 1.11.12 (#183)
- Bump github.com/sirupsen/logrus from 1.4.2 to 1.8.1 (#184)

## 1.3.0

Add rename_field flag that allows users to map fields to alternative field names.

## 1.2.0

Improvements:

- Add support for UUID parsing within lsid block to mongodb log parser.

## 1.1.5

Bug Fixes:

- Upgraded to latest version of libhoney (1.12.4) to fix a broken msgpack indirect dependency.

## 1.1.4

Bug Fixes:

- Fixed issue with bad tag for 1.1.3 causing issues with go modules. No other changes.

## 1.1.3

Bug Fixes:

- Fixed bug that was causing a panic when debug logging cli args.

## 1.1.2

Improvements:

- Add informational messaging about how the `backfill` option interacts with rate limiting.
