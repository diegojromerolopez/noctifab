FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the codebase
COPY . .

# Compile target binary and E2E test runner binary
RUN mkdir -p /dist
RUN CGO_ENABLED=0 GOOS=linux go build -o /dist/noctifab ./cmd/noctifab
RUN CGO_ENABLED=0 GOOS=linux go test -c -o /dist/e2e.test ./tests/e2e

CMD cp /dist/noctifab /shared/noctifab && cp /dist/e2e.test /shared/e2e.test && tail -f /dev/null
