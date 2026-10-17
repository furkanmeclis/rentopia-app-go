# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/create-super-admin ./cmd/create-super-admin && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/catalog-import ./cmd/catalog-import

FROM migrate/migrate:v4.19.1 AS migrate

FROM alpine:3.22 AS runtime

# Chromium + DejaVu fonts (Turkish glyphs) print contract PDFs
# (internal/platform/pdfrender). The launcher on PATH adds the flags a
# non-root container needs; chromedp finds `chromium` before
# `chromium-browser`.
RUN apk add --no-cache ca-certificates tzdata chromium font-dejavu \
    && printf '#!/bin/sh\nexec /usr/bin/chromium-browser --no-sandbox --disable-dev-shm-usage --disable-gpu "$@"\n' > /usr/local/bin/chromium \
    && chmod 0755 /usr/local/bin/chromium \
    && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/create-super-admin /app/create-super-admin
COPY --from=builder /out/catalog-import /app/catalog-import
COPY migrations /app/migrations
COPY deploy/docker/entrypoint.sh /app/entrypoint.sh

RUN mkdir -p /app/site-domains \
    && chmod +x /app/entrypoint.sh \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENV APP_HTTP_ADDR=:8080
ENV APP_ROLE=server
ENV AUTO_MIGRATE=true

ENTRYPOINT ["/app/entrypoint.sh"]
