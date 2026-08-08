# syntax=docker/dockerfile:1.6
FROM noctifab-validation:base AS noctifab-artifacts

FROM python:3.12-alpine

RUN apk add --no-cache git bash docker-cli curl build-base linux-headers

COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

WORKDIR /app
CMD ["/app/validate.sh"]
