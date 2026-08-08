FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/auth-vault main.go

FROM alpine:latest

RUN apk add --no-network --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/auth-vault /app/auth-vault

EXPOSE 8080

ENTRYPOINT ["/app/auth-vault"]
