# syntax=docker/dockerfile:1
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/web ./cmd/web

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/web /usr/local/bin/web
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/web"]
