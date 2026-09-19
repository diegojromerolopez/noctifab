# syntax=docker/dockerfile:1.6
# Per-project image for the `actodis` validation project.
# Uses `erlang:27-alpine` as the runtime base (Erlang/OTP 27 + rebar3) and
# layers noctifab + the validation projects + validate.sh from the shared base image.
#
# Build:
#   docker build -f validation/projects/actodis/Dockerfile \
#       -t noctifab-validation:actodis .

FROM noctifab-validation:base AS noctifab-artifacts

FROM erlang:27-alpine

RUN apk add --no-cache git bash docker-cli curl build-base redis

# Copy noctifab binary, validation projects, and validate.sh harness from shared base image
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable
RUN erl -eval 'erlang:display(erlang:system_info(otp_release)), halt().' -noshell && rebar3 --version

WORKDIR /app
CMD ["/app/validate.sh"]
