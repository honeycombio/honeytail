FROM golang:alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY . .
RUN go build -o /out/simple-server ./main.go

FROM scratch AS bin
WORKDIR /app
COPY --from=build /out/simple-server /app/
CMD ["/app/simple-server"]