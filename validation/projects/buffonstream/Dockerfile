FROM golang:1.22-alpine AS builder

RUN apk add --no-cache protobuf-dev protoc

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/buffonstream main.go

FROM alpine:latest

RUN apk add --no-network --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/buffonstream /app/buffonstream

EXPOSE 50051

ENTRYPOINT ["/app/buffonstream"]
