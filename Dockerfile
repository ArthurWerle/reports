# Build stage
FROM golang:1.25.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Cache bust arg - change this value to force rebuild
ARG CACHE_BUST=1

# Copy source code
COPY . .

# Build the application (static binary). Templates and migrations are embedded
# via go:embed, so nothing extra needs to be copied into the final image.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bin/server ./cmd/server

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/bin/server .

RUN chmod +x ./server

# Expose the HTTP port (actual port comes from SERVER_PORT)
EXPOSE 8080

CMD ["./server"]
