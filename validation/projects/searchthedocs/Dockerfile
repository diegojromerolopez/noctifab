# syntax=docker/dockerfile:1.6
FROM noctifab-validation:base AS noctifab-artifacts

FROM python:3.12-alpine

RUN apk add --no-cache git bash docker-cli curl build-base linux-headers postgresql-dev libffi-dev \
    && python3 -m pip install --no-cache-dir --break-system-packages \
        "fastapi>=0.110" \
        "uvicorn[standard]>=0.27" \
        "redis>=5.0" \
        "pytest>=8.0" \
        "pytest-asyncio" \
        "ruff>=0.8" \
        "mypy>=1.13" \
        "httpx" \
        "beautifulsoup4" \
        "lxml" \
        "asyncpg" \
    && rm -rf /root/.cache/pip

COPY --from=noctifab-artifacts /usr/local/bin/noctifab /usr/local/bin/noctifab
COPY --from=noctifab-artifacts /app/projects /app/projects
COPY --from=noctifab-artifacts /app/validate.sh /app/validate.sh
RUN chmod +x /app/validate.sh

WORKDIR /app
CMD ["/app/validate.sh"]
