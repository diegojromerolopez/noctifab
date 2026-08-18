# syntax=docker/dockerfile:1.6
# Per-project image for the `jpacioli` validation project.
# Uses `eclipse-temurin:21-jdk-alpine` as the runtime base (ships Java 21) and layers
# noctifab + the validation projects + validate.sh from the shared base image
# on top, plus gradle and a local PostgreSQL toolchain for integration tests.
#
# Build:
#   docker build -f validation/projects/jpacioli/Dockerfile \
#       -t noctifab-validation:jpacioli .

FROM noctifab-validation:base AS noctifab-artifacts

FROM eclipse-temurin:21-jdk-alpine

# Install git, bash, docker-cli, curl, make, gradle, and postgresql + postgresql-client
# so integration tests can spin up an ephemeral cluster as the dedicated `postgres` OS user.
RUN apk add --no-cache git bash docker-cli curl make gradle build-base postgresql postgresql-client

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN java -version && gradle --version && postgres --version && initdb --version && pg_ctl --version

WORKDIR /app
CMD ["/app/validate.sh"]
