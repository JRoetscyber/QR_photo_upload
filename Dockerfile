# ==========================================
# Multi-stage Dockerfile for Wedding Photo Server
# ==========================================

# Step 1: Build binary with Go
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile optimized static Linux binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o wedding_server .

# Step 2: Minimal runtime image (~15MB)
FROM alpine:3.20

WORKDIR /app

# Copy SSL certificates and timezones
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy compiled binary and static assets
COPY --from=builder /build/wedding_server .
COPY --from=builder /build/public ./public

# Create persistent storage directories
RUN mkdir -p /app/uploads /app/data

# Default environment variables
ENV PORT=5167
ENV ADMIN_PIN=2026

# Expose port
EXPOSE 5167

# Volumes for persistent photos & guestbook
VOLUME ["/app/uploads", "/app/data"]

# Run the server
CMD ["./wedding_server"]
