# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o kuron ./cmd/server

# Runtime stage
FROM alpine:3.20

# Install fclones from community repository
RUN apk add --no-cache fclones ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/kuron .

# Create data directory
RUN mkdir -p /data

# Environment defaults
ENV KURON_PORT=8080
ENV KURON_DB_PATH=/data/kuron.db
ENV KURON_RETENTION_DAYS=30

EXPOSE 8080

VOLUME ["/data"]

CMD ["./kuron"]
