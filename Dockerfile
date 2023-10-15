FROM golang:1.21.2-alpine3.18 AS builder

COPY . /app
RUN cd /app &&\
    go mod tidy &&\
    CGO_ENABLED=0 go build -o /tmp/external-dns-webhook-he

FROM golang:1.21.2-alpine3.18 AS final

COPY --from=builder /tmp/external-dns-webhook-he /external-dns-webhook-he

ENTRYPOINT ["/external-dns-webhook-he"]
