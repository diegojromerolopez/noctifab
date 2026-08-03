# syntax=docker/dockerfile:1.6
# Per-project image for the `notebook` validation project.
# Uses `node:22-alpine` as the runtime base (ships node 22 + npm) and layers
# noctifab + the validation projects + validate.sh from the shared base image
# on top, plus a local PostgreSQL toolchain for integration tests.
#
# Build:
#   docker build -f validation/projects/notebook/Dockerfile \
#       -t noctifab-validation:notebook .

FROM noctifab-validation:base AS noctifab-artifacts

FROM node:22-alpine

# git/bash/docker-cli/curl for the harness, build-base for native node modules,
# and postgresql + postgresql-client so integration tests can spin up an
# ephemeral cluster as the dedicated `postgres` OS user.
RUN apk add --no-cache git bash docker-cli curl build-base postgresql postgresql-client

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN node --version && npm --version && postgres --version && initdb --version && pg_ctl --version

WORKDIR /app
CMD ["/app/validate.sh"]
