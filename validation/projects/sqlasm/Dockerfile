# syntax=docker/dockerfile:1.6
FROM noctifab-validation:base AS noctifab-artifacts

FROM alpine:3.21

RUN apk add --no-cache nasm gcc musl-dev make valgrind git bash curl docker-cli

COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

WORKDIR /app
CMD ["/app/validate.sh"]
