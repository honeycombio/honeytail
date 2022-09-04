## honeytail HAProxy example

Starting with Honeycomb instrumentation "at the edge" (i.e., with your reverse
proxies or load balancers) can allow you to quickly get valuable data into
Honeycomb. You can begin gaining visibility into your systems quickly this way,
and gradually work your way inward (consequently gaining more power) by adding
native code instrumentation later.

This example demonstrates this concept by using haproxy as a reverse proxy to the
[simple server](../simple-server), and
ingesting the haproxy access logs as Honeycomb events.

## Run Natively

Run the Simple Server example linked above.

Use the provided `haproxy.cfg` for your local haproxy, and `rsyslog.conf` 
for your local rsyslog. You may need to update the following hosts in `haproxy.cfg`:
- log to `localhost` instead of `rsyslog`
- point backend app at `localhost` instead of `api`

Then:

```
$ honeytail --debug \
    --parser=nginx \
    --dataset=examples.honeytail.haproxy \
    --writekey=$HONEYCOMB_WRITE_KEY \
    --nginx.conf=hny-haproxy.conf \
    --nginx.format=haproxy \
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

| **Name**                   | **Description**                                                    | **Example Value**            |
|----------------------------|--------------------------------------------------------------------|------------------------------|
| `act_conn`                 | # of active connections                                            | `1`                          |
| `backend`                  | Proxy backend name                                                 | `app`                        |
| `backend_queue`            | # requests processed ahead of current request on the backend queue | `0`                          |
| `backend_server`           | Proxy backend server name                                          | `simpleServer`               |
| `be_conn`                  | # active backend connections                                       | `0`                          |
| `bytes_read`               | # bytes sent to the client                                         | `341`                        |
| `client_ip`                | IP address of the client which initiated the connection to haproxy | `172.21.0.1`                 |
| `client_port`              | TCP port of the client which initiated the connection to haproxy   | `59240`                      |
| `fe_conn`                  | # active frontend connections                                      | `1`                          |
| `frontend`                 | Proxy frontend name                                                | `main`                       |
| `hostname`                 | Proxy hostname                                                     | `localhost`                  |
| `pid`                      | Proxy pid                                                          | `7`                          |
| `process`                  | Proxy process                                                      | `haproxy`                    |
| `request`                  | Complete HTTP request line                                         | `GET /hello/meow HTTP/1.1`   |
| `request_headers`          | Captured request headers                                           | `localhost&#x7c;curl/7.79.1` |
| `request_method`           | HTTP request method                                                | `GET`                        |
| `request_path`             | URL of the request                                                 | `/hello/meow`                |
| `request_pathshape`        | "Shape" of the request path                                        | `/hello/meow`                |
| `request_protocol_version` | HTTP version                                                       | `HTTP/1.1`                   |
| `request_shape`            | "Shape" of the request                                             | `/hello/meow`                |
| `request_uri`              | URI for the request                                                | `/hello/meow`                |
| `response_headers`         | Captured response headers                                          | `&#x7c;text/plain; charset=` |
| `retries`                  | # connection retries                                               | `0`                          |
| `srv_conn`                 | # active server connections                                        | `0`                          |
| `srv_queue`                | # requests processed ahead of current request on the server queue  | `0`                          |
| `status_code`              | HTTP status code returned                                          | `200`                        |
| `termination_state`        | Session state at disconnection                                     | `----`                       |
| `time_backend_conn`        | Total time (ms) waiting for connection to final server             | `0`                          |
| `time_backend_resp`        | Total time (ms) waiting for a full request from the backend        | `5`                          |
| `time_client_connect`      | Total time (ms) waiting for a full request from the client         | `0`                          |
| `time_queued`              | Total time (ms) waiting in proxy queues                            | `0`                          |
| `time_total`               | Total time (ms) the request remained active in haproxy             | `5`                          |
