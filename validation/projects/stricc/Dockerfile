# syntax=docker/dockerfile:1.6
# Per-project image for the `stricc` validation project.
# Safe, defined-behavior C compiler written in Rust targeting LLVM 18.
# Validates against GCC and Clang test suites.

FROM noctifab-validation:base AS noctifab-artifacts

FROM rust:alpine

RUN rustup component add rustfmt clippy

RUN apk add --no-cache \
    git \
    bash \
    docker-cli \
    curl \
    build-base \
    make \
    gcc \
    clang \
    llvm18-dev \
    llvm18-static \
    clang-dev

ENV LLVM_SYS_180_PREFIX=/usr

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN cargo --version && gcc --version && clang --version

WORKDIR /app
CMD ["/app/validate.sh"]
