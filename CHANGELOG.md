# Honeytail Changelog

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
