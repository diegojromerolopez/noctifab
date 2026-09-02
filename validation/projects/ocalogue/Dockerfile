# syntax=docker/dockerfile:1.6
FROM noctifab-validation:base AS noctifab-artifacts

FROM ocaml/opam:alpine-ocaml-5.2

USER root
RUN apk add --no-cache git bash docker-cli curl make gcc musl-dev ocaml opam dune

COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh
RUN mkdir -p /app && chown -R opam:opam /app

USER opam
ENV OPAMROOT=/home/opam/.opam
ENV OPAM_SWITCH_PREFIX=/home/opam/.opam/5.2
ENV PATH="/home/opam/.opam/5.2/bin:${PATH}"

WORKDIR /app
CMD ["/app/validate.sh"]

