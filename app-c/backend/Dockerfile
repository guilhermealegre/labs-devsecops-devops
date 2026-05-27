# =============================================================================
# App C Backend - Rust Multi-stage Dockerfile
# =============================================================================
FROM rust:1.75-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache musl-dev pkgconfig openssl-dev

# Copy cargo files for dependency caching
COPY Cargo.toml Cargo.lock* ./

# Create a dummy main.rs to build dependencies
RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo build --release
RUN rm -rf src

# Copy actual source code
COPY src ./src

# Build the application
RUN touch src/main.rs && cargo build --release

# =============================================================================
# Final stage - minimal image
# =============================================================================
FROM alpine:3.19

RUN apk --no-cache add ca-certificates libgcc

WORKDIR /app

COPY --from=builder /app/target/release/app-c-backend .

EXPOSE 8080

CMD ["./app-c-backend"]
