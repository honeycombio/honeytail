# honeytail

[![CircleCI](https://circleci.com/gh/honeycombio/honeytail.svg?style=shield)](https://circleci.com/gh/honeycombio/honeytail)
[![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/honeytail)](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)

`honeytail` is [Honeycomb](https://honeycomb.io)'s agent for ingesting log file data into Honeycomb and making it available for exploration. Its favorite format is **JSON**, but understands how to parse a range of other well-known log formats.

See [our documentation](https://honeycomb.io/docs/send-data/agent/) to read about how to configure and run `honeytail`, to find tips and best practices, and to download prebuilt versions.

## Supported Parsers

`honeytail` supports reading files from `STDIN` as well as from a file on disk.

Our complete list of parsers can be found in the [`parsers/` directory](parsers/), but as of this writing, `honeytail` will support parsing logs generated by:

- [ArangoDB](parsers/arangodb/)
- [MongoDB](parsers/mongodb/)
- [MySQL](parsers/mysql/)
- [PostgreSQL](parsers/postgresql/) (Note: does not support quoted table or column names in queries.)
- [nginx](parsers/nginx/)
- [regex](parsers/regex/)
- [keyval](parsers/keyval/)([logfmt](https://brandur.org/logfmt))
- [csv](parsers/csv/)
- [syslog](parsers/syslog/)

## Installation

Install from source:

```
go get github.com/honeycombio/honeytail
```

to install to a specific path:

```
GOPATH=/usr/local go get github.com/honeycombio/honeytail
```

the binary will install to `/usr/local/bin/honeytail`

Use a prebuilt binary: find the latest version on [Honeycomb.io](https://honeycomb.io/docs/send-data/agent/)

## Usage

```
honeytail --writekey=YOUR_WRITE_KEY --dataset='Best Data Ever' --parser=json --file=/var/log/api_server.log
```

For more advanced usage, options, and the ability to scrub or drop specific fields, see [our documentation](https://honeycomb.io/docs/send-data/agent).

## Related Work

In some cases, we've extracted out some generic work for a particular log format

- [mongodbtools](https://github.com/honeycombio/mongodbtools) contains logic specific to parsing various versions of MongoDB logs, and a script for capturing high-level statistics on the database server itself
- [mysqltools](https://github.com/honeycombio/mysqltools) contains logic specific to normalizing MySQL queries

## Contributions

Features, bug fixes and other changes to honeytail are gladly accepted. Please
open issues or a pull request with your change. Remember to add your name to the
CONTRIBUTORS file!

All contributions will be released under the Apache License 2.0.
