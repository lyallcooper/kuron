# Build stage
FROM golang:1.25-alpine AS builder

# Version info - pass via --build-arg
ARG VERSION=dev
ARG COMMIT=unknown

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for SQLite, injecting version info
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o kuron ./cmd/server

# Runtime stage
FROM alpine:3.20

# Install fclones from community repository (pinned for reproducibility)
RUN apk add --no-cache fclones=0.34.0-r0 ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/kuron .

# Create data directory
RUN mkdir -p /data

ENV KURON_DB_PATH=/data/kuron.db

EXPOSE 8080

VOLUME ["/data"]

CMD ["./kuron"]
