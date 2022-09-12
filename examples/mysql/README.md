## honeytail mysql

This example demonstrates using [Honeytail to ingest the MySQL slow query
log](https://docs.honeycomb.io/getting-data-in/integrations/databases/mysql/logs/).

The `HONEYCOMB_WRITE_KEY` environment variable must be set to your Honeycomb
write key. To run the example, just `docker-compose up`. There will likely be a
few error messages as downstream containers restart because MySQL takes a second
to start up fully.

## Architecture

There are three parts to this example:

1. A MySQL install
2. A Honeytail instance which tails the MySQL slow query log, parsing it into
   structured events and forwarding them to Honeycomb for analysis
3. A `mysqlslap` container which generates load so the slow query log actually
   has some stuff in it

Once running, you can leave it for a while, and you will be able to ask questions
in Honeycomb like which queries are slowest, grouped by `normalized_query`,
`client`, `user`, and so on.

You can even use BubbleUp to dive into details about where you might want to
explore next when particular queries are slow, or simply odd.

Note the usage of `--drop_field=query` as a flag for Honeytail. This will
ensure that the `query` field (which contains the raw, non-normalized query) is
not sent, which might otherwise expose sensitive details from the queries your
apps are running. For most insights you need to divine, `normalized_query`
(which will show a generic form of query like `select * from users where id =
?`) is perfectly sufficient.
