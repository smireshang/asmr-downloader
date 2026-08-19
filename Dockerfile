# =========================
# Build stage
# =========================

FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod ./

COPY . .

RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=${TARGETARCH} \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /asmr-downloader \
    ./cmd/server


# =========================
# Runtime stage
# =========================

FROM debian:bookworm-slim

WORKDIR /app

# 安装 CA 证书
# 用于 Go HTTPS 请求验证远程服务器 TLS 证书
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder \
    /asmr-downloader \
    /app/asmr-downloader

COPY web \
    /app/web

RUN mkdir -p \
    /data/downloads

ENV PORT=8080

ENV DOWNLOAD_DIR=/data/downloads

ENV ASMR_API_HOST=https://api.asmr-200.com

EXPOSE 8080

CMD ["/app/asmr-downloader"]