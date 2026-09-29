# syntax=docker/dockerfile:1.6
# Per-project image for the `thredis` validation project.
# Uses `mcr.microsoft.com/dotnet/sdk:9.0-alpine` as the runtime base (.NET 9 SDK) and
# layers noctifab + the validation projects + validate.sh from the shared base image.
#
# Build:
#   docker build -f validation/projects/thredis/Dockerfile \
#       -t noctifab-validation:thredis .

FROM noctifab-validation:base AS noctifab-artifacts

FROM mcr.microsoft.com/dotnet/sdk:9.0-alpine

RUN apk add --no-cache git bash docker-cli curl build-base make redis

# Copy noctifab binary, validation projects, and validate.sh harness from shared base image
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable
RUN dotnet --version

WORKDIR /app
CMD ["/app/validate.sh"]
