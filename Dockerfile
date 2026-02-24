# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Download dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /app/server ./cmd/server

# ─── Final image ───────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runner

WORKDIR /app

# Runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary and static assets
COPY --from=builder /app/server ./server
COPY --from=builder /app/public ./public
COPY --from=builder /app/internal/migrations/sql ./internal/migrations/sql

# Non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup \
    && chown -R appuser:appgroup /app

USER appuser

ENV PORT=8080
EXPOSE 8080

CMD ["./server"]
