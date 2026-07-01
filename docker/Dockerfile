FROM golang:1.25-alpine AS go-builder

WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN VERSION=$(cat VERSION) && \
    CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o caddydash .

FROM alpine:latest AS BUILD

ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM

RUN apk update && apk add --no-cache sed wget curl tar

RUN mkdir -p /data/caddy/config
RUN mkdir -p /data/caddy/config.d
RUN mkdir -p /usr/local/bin

RUN VERSION=$(curl -s https://raw.githubusercontent.com/WJQSERVER/caddy/main/TEST-VERSION) && \
    echo "VERSION: ${VERSION}" && \
    wget -O /data/caddy/caddy.tar.gz https://github.com/WJQSERVER/caddy/releases/download/${VERSION}/caddy-${TARGETOS}-${TARGETARCH}-pages.tar.gz
RUN tar -xzvf /data/caddy/caddy.tar.gz -C /data/caddy 
RUN rm /data/caddy/caddy.tar.gz 
RUN chmod +x /data/caddy/caddy

COPY --from=go-builder /build/caddydash /data/caddy/caddydash
RUN chmod +x /data/caddy/caddydash

FROM alpine:latest

RUN mkdir -p /data/caddy/config 
RUN mkdir -p /data/caddy/config.d
RUN mkdir -p /data/caddy/log
RUN mkdir -p /data/caddy/db
RUN mkdir -p /data/caddy/frontend
RUN mkdir -p /data/caddy/gtmpl
RUN mkdir -p /data/caddy/tmpl

COPY --from=BUILD /data/caddy /data/caddy

COPY frontend /data/caddy/frontend/
COPY gtmpl /data/caddy/gtmpl/
COPY tmpl /data/caddy/tmpl/

RUN chmod +x /data/caddy/caddy 
RUN chmod +x /data/caddy/caddydash

WORKDIR /data/caddy/
ENTRYPOINT ["/data/caddy/caddydash", "-c", "/data/caddy/config/config.toml"]