# Build
FROM golang:1.23-alpine AS builder
ENV GOTOOLCHAIN=local
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6 && \
    swag init -g cmd/server/main.go -o docs --parseInternal
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /server /app/server
COPY --from=builder /app/internal/db/migrations /app/migrations
EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/server"]
