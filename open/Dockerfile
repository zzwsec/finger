# syntax=docker/dockerfile:1

FROM golang:1.27.0-alpine3.24 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/open ./cmd/open

FROM alpine:3.24

RUN apk add --no-cache ansible-core openssh-client tini tzdata

ENV TZ=Asia/Shanghai \
    ANSIBLE_CONFIG=/open/automation/ansible.cfg

WORKDIR /open

COPY --from=builder /out/open /usr/local/bin/open
COPY automation ./automation

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/open"]
