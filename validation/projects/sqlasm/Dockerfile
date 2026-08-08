FROM --platform=linux/amd64 amd64/debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    nasm \
    gcc \
    libc6-dev \
    make \
    valgrind \
    gdb \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .

RUN make build || true

ENTRYPOINT ["/app/bin/sqlasm"]
