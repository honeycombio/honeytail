## honeytail nginx example

Starting with Honeycomb instrumentation "at the edge" (i.e., with your reverse
proxies or load balancers) can allow you to quickly get valuable data into
Honeycomb. You can begin gaining visibility into your systems quickly this way,
and gradually work your way inward (consequently gaining more power) by adding
native code instrumentation later.

This example demonstrates this concept by using nginx as a reverse proxy to the
[simple server](../simple-server), and
ingesting the nginx access logs as Honeycomb events.

## Run Natively

Run the Python API example linked above.

Use the provided `nginx.conf` for your local nginx config. You may need to
update the `proxy_pass` to pass to `localhost` instead of `api`. Then:

```
$ honeytail --debug \
    --parser=nginx \
    --dataset=examples.honeytail-nginx \
    --writekey=$HONEYCOMB_WRITE_KEY \
    --nginx.conf=/etc/nginx/nginx.conf \
    --nginx.format=honeytail \
    --file=/var/log/honeytail/access.log
```

## Run in Docker

```shell
docker-compose build && docker-compose up -d
curl localhost
> I'm here!
curl localhost/hello/meow
> Hello meow!
```

## Event Fields

| **Name**                   | **Description**                                         | **Example Value**          |
|----------------------------|---------------------------------------------------------|----------------------------|
| `body_bytes_sent`          | # of bytes in the HTTP response body sent to the client | 157                        |
| `bytes_sent`               | # of bytes sent to the client                           | 24                         |
| `host`                     | Hostname of the nginx server responding to the request  | `localhost`                |
| `http_user_agent`          | User agent of the request                               | `curl/7.79.1`              |
| `remote_addr`              | Client address                                          | `172.19.0.1`               |
| `request`                  | Full original request line                              | `GET /hello/meow HTTP/1.1` |
| `request_length`           | Size of request in bytes                                | 456                        |
| `request_method`           | HTTP request method                                     | `GET`                      |
| `request_path`             | URL of the request                                      | `/hello/meow`              |
| `request_pathshape`        | "Shape" of the request path                             | `/hello/meow`              |
| `request_protocol_version` | HTTP version                                            | `HTTP/1.1`                 |
| `request_shape`            | "Shape" of the request                                  | `/hello/meow`              |
| `request_time`             | Amount of time it took to serve the request in seconds  | 250                        |
| `request_uri`              | URI for the request                                     | `/hello/meow`              |
| `server_name`              | Name of the server serving the request                  | `localhost`                |
| `status`                   | HTTP status code returned                               | 200                        |
