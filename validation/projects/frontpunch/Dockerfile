# syntax=docker/dockerfile:1.6
# Per-project image for the `frontpunch` validation project.
# Builds on top of noctifab-validation:base and layers the Python toolchain
# on top: python3, pip, and the runtime/test deps pinned by SPEC §2.6
# (redis, fastapi, uvicorn, cryptography, croniter, click) plus the
# formatter/linter (black, ruff, mypy) and coverage.
#
# Build:
#   docker build -f validation/projects/frontpunch/Dockerfile \
#       -t noctifab-validation:frontpunch .

FROM noctifab-validation:base

RUN apk add --no-cache python3 py3-pip build-base libffi-dev openssl-dev \
    && python3 -m pip install --no-cache-dir --break-system-packages \
        "redis>=5.0,<6.0" \
        "fastapi>=0.110,<0.120" \
        "uvicorn[standard]>=0.27" \
        "cryptography>=42.0" \
        "croniter>=2.0" \
        "click>=8.1" \
        "black" \
        "ruff" \
        "mypy" \
        "coverage" \
    && rm -rf /root/.cache/pip

WORKDIR /app
CMD ["/app/validate.sh"]