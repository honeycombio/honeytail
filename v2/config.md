## Honeytail "--config-file" format

The file format is JSON/[JSON5](https://github.com/json5/json5/blob/master/README.md#features).

```json5
{
    source: { // one of...
        "files": [paths...]
    },

    parser: {
        engine: { // one of...
            "nginx": {
                config_file = path to nginx config file
                // An Nginx config file can define multiple named log
                // formats.  Use this to specify the name of the log
                // format your source data is using.
                log_format_name = string
            }
            "mysql": {
                // No configuration options.
            },
        },

        // The number of parser threads to spawn.
        num_threads: int
    },

    // Post-processing of the event before uploading.
    // NOTE: The filtering code isn't implemented yet.  I'm just
    // sketching out one idea for the configuration format.
    filter: {
        // A list of rules.  The rule types are documented below.
        rules: [
            // Add a field with the given value.
            ["add", field_name, field_value],
            
            // Drop a field.
            ["drop", field_name],

            // Replace a field's value with the SHA-256 hash of its value.
            ["sha256", field_name],

            // Parse a field as an HTTP request line and produce new
            // fields for each component of the request line.  If the
            // field name/value is "xyz"/"GET /blah?x=12 HTTP/1.1",
            // you'll get a few additional fields:
            //     "<field_name>_method": "GET"
            //     "<field_name>_protocol_version": "1.1"
            //     "<field_name>_uri": "/blah?x=12"
            //     "<field_name>_path": "/blah"
            //     "<field_name>_query": "x=12"
            //     "<field_name>_query_x": ""
            ["parse_request_line", field_name, {
                // Add a prefix to the output field name, to avoid
                // collisions.
                "field_name_prefix": string
                "path_patterns": [
                    "blah",
                ],
                // Which query params to include.
                "query_params": // one of..
                    // Only include query params from the list.
                    | {"whitelist": [query_param_names...]},
                    // Include all query params.
                    | "all",
            }]

            // Sample different categories of events differently.  The
            // sample rate for each category is based on their frequency
            // in the last N seconds of data.
            ["dynamic_sample", [field_names...], {
                // The overall sample rate to aim for.
                "goal_rate": int
                // The size, in seconds, of the dynamic sampling window.
                // Defaults to 30.
                "window_sec": int
            }]
        ]
    },

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

The file format is JSON/[JSON5](https://github.com/json5/json5/blob/master/README.md#features).

```
{
    write_key: string,
    api_url: string,
        // Optional; defaults to "https://api.honeycomb.io/"
}
```
