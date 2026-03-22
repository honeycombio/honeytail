# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY . .

RUN ver=$(git describe --tags --match='v[0-9]*' --always) \
    && go build -ldflags="-X main.BuildID=${ver}" -o /honeytail .

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /honeytail /usr/local/bin/honeytail

ENV HONEYCOMB_WRITE_KEY=NULL
ENV NGINX_LOG_FORMAT_NAME=combined
ENV NGINX_CONF=/etc/nginx.conf
ENV HONEYCOMB_SAMPLE_RATE=1
ENV NGINX_ACCESS_LOG_FILENAME=access.log

CMD [ "/bin/sh", "-c", "honeytail \
            --parser nginx \
            --writekey $HONEYCOMB_WRITE_KEY \
            --file /var/log/nginx/$NGINX_ACCESS_LOG_FILENAME \
            --dataset nginx \
            --samplerate $HONEYCOMB_SAMPLE_RATE \
            --nginx.conf $NGINX_CONF \
            --nginx.format $NGINX_LOG_FORMAT_NAME \
            --tail.read_from end" ]
