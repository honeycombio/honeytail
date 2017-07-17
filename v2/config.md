## Honeytail "--config-file" format

Can be a ".json" or [".json5"](https://github.com/json5/json5/blob/master/README.md#features) file.

```json5
{
    source: { // one of...
        "files": [paths...]
    },

    parser: {
        engine: { // one of...
            "nginx": {
                config_file: path to nginx config file
                // An Nginx config file can define multiple named log
                // formats.  Use this to specify the name of the log
                // format your source data is using.
                log_format_name: string
            }
            "mysql": {
                // No configuration options.
            },
        },

        // The number of parser threads to spawn.  NOTE: Any filtering rules are applied
        // on the parser threads as well.
        num_threads: int

        // Do sampling in the parser.  If set to 5, we will select approximately 1 out
        // of every five log entries.  The selection will be done with a pseudo-random
        // number generator.
        sample_rate: int
    },

    // A list of rules to apply after parsing.  The rule types are documented below.
    filter: [
        // Add a field with the given value.
        ["add", field_name, field_value],

        // Drop a field.
        ["drop", field_name],

        // Replace a field's value with the SHA-256 hash of its value.
        ["sha256", field_name],

        // Parse a field as an HTTP request line and produce new fields
        // for each component of the request line.
        // See: https://github.com/honeycombio/urlshaper
        ["parse_request_line", field_name, {
            // Add a prefix to the output field name, to avoid
            // collisions.
            "field_name_prefix": string

            // URL path patterns, e.g. "/about/:lang/books/:isbn"
            "path_patterns": [patterns...],

            // Which query params to copy into top-level fields.
            "extract_query_params": // one of..
                // Only extract query params from the list.
                | [query_param_names...],
                // Extract all query params.
                | "all",
        }]

        // Sample different categories of events differently.  The
        // sample rate for each category is based on their frequency
        // in the last N seconds of data.
        ["dynamic_sample", goal_rate, [field_names...], {
            // The size, in seconds, of the dynamic sampling window.
            // Defaults to 30.
            window_sec: int
            // If the rate falls below this, dynsampler won't sample.
            // Defaults to 1.
            min_rate: int
        }]
    ],

    // Not needed in "--test" mode.
    uploader: {
        data_set: string,
            // Name of Honeycomb data set to upload events to.

        libhoney_config: {  // Optional
            max_concurrent_batches: int,
            send_frequency_ms: int,
            max_batch_size: int,
        },
    },
}
```

## Honeytail "--write-key-file" format

Can be a ".json" or [".json5"](https://github.com/json5/json5/blob/master/README.md#features) file.

```json5
{
    write_key: string,

    // Optional; defaults to "https://api.honeycomb.io/"
    api_url: string,
}
```
