# =============================================================================
# App B Worker - Go Multi-stage Dockerfile
# =============================================================================
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o worker .

# =============================================================================
# Final stage - minimal image
# =============================================================================
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/worker .

EXPOSE 8080

CMD ["./worker"]
