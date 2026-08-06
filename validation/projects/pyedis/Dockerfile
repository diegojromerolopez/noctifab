# syntax=docker/dockerfile:1.6
# Per-project image for the `pyedis` validation project.
# Uses `python:3.14-alpine` as the runtime base (ships python 3.14 + pip) and
# layers noctifab + the validation projects + validate.sh from the shared base
# image on top, plus the pinned FastAPI/test/lint toolchain.
#
# Build:
#   docker build -f validation/projects/pyedis/Dockerfile \
#       -t noctifab-validation:pyedis .

FROM noctifab-validation:base AS noctifab-artifacts

FROM python:3.14-alpine

RUN apk add --no-cache git bash docker-cli curl build-base

# Pre-install the project's runtime + dev toolchain so package resolution does
# not depend on runtime network access.
RUN python3 -m pip install --no-cache-dir --break-system-packages \
        "fastapi>=0.115" \
        "pydantic>=2.10" \
        "uvicorn[standard]>=0.30" \
        "pytest>=8.0" \
        "pytest-asyncio>=0.23" \
        "httpx>=0.27" \
        "ruff>=0.8" \
        "mypy>=1.13" \
        "coverage>=7.6" \
        "pytest-cov>=5.0" \
    && rm -rf /root/.cache/pip

# Copy noctifab binary, validation projects, and the validate.sh harness
# from the shared base image.
COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

# Sanity check the toolchain is callable.
RUN python3 --version && python3 -c "import fastapi, uvicorn, pydantic; print('fastapi', fastapi.__version__)" && ruff --version && mypy --version

WORKDIR /app
CMD ["/app/validate.sh"]
