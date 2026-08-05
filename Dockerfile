# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO disabled for static linking
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o forge .

# Final stage
FROM alpine:3.18

WORKDIR /root/

# Install ca-certificates for HTTPS, tzdata for timezones, and wget for healthchecks
RUN apk --no-cache add ca-certificates tzdata wget

# Create temp directory for file uploads
RUN mkdir -p /tmp/forge && chmod 755 /tmp/forge

# Copy the binary from builder
COPY --from=builder /app/forge .

# Copy .env.example as default config (optional)
# COPY .env.example .env

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./forge"]
