# syntax=docker/dockerfile:1.6
# Per-project image for the `wc` validation project.
# Uses `rust:1.84-alpine` as the runtime base (ships rustc, cargo, rustfmt,
# clippy in one image, no rustup dance) and layers noctifab + the validation
# projects + validate.sh from the shared base image on top.
#
# Build (run_one.sh does this automatically):
#   docker build -f validation/projects/wc/Dockerfile \
#       -t noctifab-validation:wc .

FROM noctifab-validation:base AS noctifab-artifacts
# No-op stage: just gives us a name to COPY --from below.

FROM rust:1.84-alpine

RUN rustup component add rustfmt clippy

RUN apk add --no-cache git bash docker-cli

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN cargo --version && cargo fmt --version && cargo clippy --version

WORKDIR /app
CMD ["/app/validate.sh"]