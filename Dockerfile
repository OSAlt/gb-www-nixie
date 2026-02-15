# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache make git

# Copy go mod and sum files
COPY go.mod go.sum ./
COPY tools/go.mod tools/go.sum ./tools/

# Download dependencies
RUN go mod download
RUN cd tools && go mod download

# Copy the rest of the source code
COPY . .

# Install templ and generate files
RUN go run -modfile tools/go.mod github.com/a-h/templ/cmd/templ generate
RUN go run -modfile tools/go.mod github.com/sqlc-dev/sqlc/cmd/sqlc generate

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Final stage
FROM alpine:latest

WORKDIR /

# Copy the binary from the builder stage
COPY --from=builder /server /server

# Copy static assets and migrations
COPY --from=builder /app/static /static
COPY --from=builder /app/db/migrations /db/migrations
COPY --from=builder /app/tools /tools

# Expose the port the app runs on
EXPOSE 8080

# Command to run the application
CMD ["/server"]
