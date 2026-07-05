# syntax=docker/dockerfile:1.6
# Per-project image for the `todo-cli` validation project.
# Uses `golang:alpine` as the runtime base and layers noctifab + the validation
# projects + validate.sh from the shared base image on top.
#
# Build:
#   docker build -f validation/projects/todo-cli/Dockerfile \
#       -t noctifab-validation:todo-cli .

FROM noctifab-validation:base AS noctifab-artifacts

FROM golang:alpine

RUN apk add --no-cache git bash docker-cli

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN go version

WORKDIR /app
CMD ["/app/validate.sh"]