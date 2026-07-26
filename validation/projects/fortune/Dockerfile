# syntax=docker/dockerfile:1.6
# Per-project image for the `fortune` validation project.
# Uses Alpine with GCC and SQLite3 libraries as runtime base.

FROM noctifab-validation:base AS noctifab-artifacts

FROM alpine:latest

# Install C build dependencies, gcc, sqlite-dev, make, git, bash, docker-cli, curl
RUN apk add --no-cache gcc musl-dev make sqlite-dev sqlite git bash docker-cli curl

# Copy noctifab binary, validation projects, and the validate.sh harness
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check toolchain
RUN gcc --version && make --version && sqlite3 --version

WORKDIR /app
CMD ["/app/validate.sh"]
