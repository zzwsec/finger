# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.24.0-alpine3.21 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg \
    go mod download
ARG TARGETOS
ARG TARGETARCH
ARG CACHE_ID=apk-cache-builder
RUN --mount=type=cache,id=${CACHE_ID}-${TARGETARCH},target=/var/cache/apk,sharing=locked \
    ln -s /var/cache/apk /etc/apk/cache \
    && apk update \
    && apk add --no-progress tzdata
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/open-linux .

FROM alpine:3.21.3
WORKDIR /open
ARG CACHE_ID=apk-cache
ARG TARGETARCH
COPY --from=builder /out/open-linux /open/open-linux
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
COPY ansible.cfg /open
COPY playbook /open/playbook/
RUN --mount=type=cache,id=${CACHE_ID}-${TARGETARCH},target=/var/cache/apk,sharing=locked \
    ln -s /var/cache/apk /etc/apk/cache \
    && apk update \
    && apk add --no-progress ansible openssh-client \
    && echo Asia/Shanghai > /etc/timezone \
    && mkdir -p /var/log/ansible

ENTRYPOINT ["/open/open-linux"]