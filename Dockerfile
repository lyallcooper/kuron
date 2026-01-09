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
FROM alpine:3.23

# Install fclones from community repository (pinned for reproducibility)
RUN apk add --no-cache fclones=0.35.0-r0 ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/kuron .

# Create data directory (fclones stores cache in $HOME/.cache/fclones)
RUN mkdir -p /data /data/.cache

ENV KURON_DB_PATH=/data/kuron.db
# Set HOME so fclones can write to ~/.cache/fclones
ENV HOME=/data

EXPOSE 8080

VOLUME ["/data"]

CMD ["./kuron"]
