# honeytail example

There is an example log file (`honeytail-example.log`) to see how this would show up in Honeycomb.
We are using the `--backfill` option to get the existing logs.

Update the date/time to more easily view in Honeycomb.
For example, `time="2021-11-24T03:41:04Z"` is November 24, 2021.
If today is January 7, 2022, change this to `time="2022-01-07T03:41:04Z"` and look at 24 hours timeframe in Honeycomb (depending on how close it is to current time).
Make sure the time is in the past, not the future.

## Setup and Run Locally

```shell
go install github.com/honeycombio/honeytail@latest
```

Set your `HONEYCOMB_API_KEY` environment variable if running locally with command line arguments, or update the API Key in config

### Using command line arguments

```shell
honeytail --debug \
    --writekey=$HONEYCOMB_API_KEY \
    --dataset='honeytail-example' \
    --parser=keyval \
    --keyval.timefield=time \
    --file=./honeytail-example.log \
    --backfill
```

### Using a configuration file

```shell
honeytail -c ./honeytail-example.conf
```
