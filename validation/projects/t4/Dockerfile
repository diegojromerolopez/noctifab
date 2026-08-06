# syntax=docker/dockerfile:1.6
# Per-project image for the `t4` validation project.
# Uses `alpine:3.21` as the runtime base (gcc/clang/make from apk) and layers
# noctifab + the validation projects + validate.sh from the shared base image.
#
# Build:
#   docker build -f validation/projects/t4/Dockerfile \
#       -t noctifab-validation:t4 .

FROM noctifab-validation:base AS noctifab-artifacts

FROM alpine:3.21

# Build toolchain (gcc, make), formatter/linter (clang-format, clang-tidy), and
# the harness tools (curl for black-box tests, docker-cli).
RUN apk add --no-cache git bash docker-cli curl build-base clang clang-extra-tools coreutils

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN gcc --version && make --version && clang-format --version

WORKDIR /app
CMD ["/app/validate.sh"]
