# syntax=docker/dockerfile:1.6
# Per-project image for the `calculator` validation project.
# Uses `ruby:3.2-alpine` as the runtime base and layers noctifab + the validation
# projects + validate.sh from the shared base image on top.

FROM noctifab-validation:base AS noctifab-artifacts

FROM ruby:3.2-alpine

# Install build dependencies, git, bash, docker-cli
RUN apk add --no-cache git bash docker-cli build-base

# Pre-install rubocop and rspec gems so they are cached/available in the image
RUN gem install rspec rubocop

# Copy noctifab binary, validation projects, and the validate.sh harness
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable
RUN ruby --version && rspec --version && rubocop --version

WORKDIR /app
CMD ["/app/validate.sh"]
