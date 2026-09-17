# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24.6

FROM golang:${GO_VERSION}-bookworm AS dev
RUN apt-get update \
  && apt-get install -y --no-install-recommends bash git jq make ca-certificates \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["bash"]

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags "-s -w -X github.com/mnemon-dev/mnemon/cmd.version=${VERSION}" \
  -o /out/mnemon . \
  && CGO_ENABLED=0 GOOS=linux go build \
  -ldflags "-s -w" \
  -o /out/mnemon-server ./cmd/mnemon-server

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S mnemon \
  && adduser -S -G mnemon -h /home/mnemon mnemon \
  && mkdir -p /mnemon \
  && chown -R mnemon:mnemon /mnemon /home/mnemon
COPY --from=build /out/mnemon /usr/local/bin/mnemon
USER mnemon
ENV MNEMON_DATA_DIR=/mnemon \
    MNEMON_STORE=default
VOLUME ["/mnemon"]
ENTRYPOINT ["mnemon"]
CMD ["status"]

FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S mnemon \
  && adduser -S -G mnemon -h /home/mnemon mnemon \
  && mkdir -p /data \
  && chown -R mnemon:mnemon /data /home/mnemon
COPY --from=build /out/mnemon-server /usr/local/bin/mnemon-server
USER mnemon
ENV MNEMON_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 7443
ENTRYPOINT ["mnemon-server"]
CMD ["serve"]
