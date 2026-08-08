# syntax=docker/dockerfile:1.6
FROM noctifab-validation:base AS noctifab-artifacts

FROM rust:alpine

RUN apk add --no-cache git bash docker-cli curl gcc make clang llvm18

COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

WORKDIR /app
CMD ["/app/validate.sh"]
