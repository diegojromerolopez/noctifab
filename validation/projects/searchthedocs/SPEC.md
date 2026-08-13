# searchthedocs Specification: Python ReadTheDocs RAG Search Engine (DDD, SOLID & OpenAI-Compatible LLM Integration)

## 1. Overview

`searchthedocs` is a high-performance RAG (Retrieval-Augmented Generation) documentation search platform written in **Python 3.15+ (FastAPI)**. It allows users to submit any ReadTheDocs URL (e.g. `https://<subdomain>.readthedocs.io/`), enqueues an asynchronous scraping job via **Redis / Valkey**, extracts documentation pages using background worker processes (`BeautifulSoup4` + `lxml` + `httpx`), indexes chunks into a vector database with dense vector embeddings (**PostgreSQL `pgvector`**), and serves semantic RAG search queries and synthesized LLM answers via an **OpenAI-compatible LLM interface**, modern web UI, and REST API.

The project tests key Dark Factory engineering seams:
- **Domain-Driven Design (DDD) & SOLID Seam:** Clean boundary isolation between Domain models (`domain/`), Application use-case services (`application/`), Infrastructure technology adapters (`infrastructure/`), and API controllers (`api/`) with strict **Dependency Injection (DI)**.
- **Asynchronous Queue & Worker Seam:** Decoupled job submission (`POST /api/v1/jobs`) from background scraping workers using Redis lists.
- **RAG Vector Search & OpenAI LLM Seam:** Sliding-window text chunking, dense vector embeddings via `/v1/embeddings` or local deterministic hashing, HNSW vector retrieval via `pgvector`, and optional RAG answer synthesis via `/v1/chat/completions`.
- **Full-Stack Web Interface & API:** Embedded modern single-page search dashboard and OpenAPI REST endpoints.

> [!IMPORTANT]
> **Toolchain & Standards Mandate**: `searchthedocs` MUST be implemented in **Python 3.15+**. Code MUST pass `ruff check .` with ZERO warnings and strict `mypy --strict` compliance. Every source file MUST remain under 500 lines of code.
>
> **Domain-Driven Design (DDD) Mandate**: The codebase MUST strictly adhere to DDD layer boundaries. Business logic and interfaces MUST reside in `src/domain/`. Application orchestrations MUST reside in `src/application/`. External I/O, database access, network scraping, and LLM clients MUST reside in `src/infrastructure/`. Route handlers and DI factories MUST reside in `src/api/`.
>
> **SOLID & Dependency Injection Mandate**: Code MUST strictly adhere to SOLID principles. Domain interfaces MUST be defined as abstract `typing.Protocol` classes. All application services MUST receive their dependencies via constructor injection. FastAPI route handlers MUST NOT instantiate infrastructure clients directly; dependencies MUST be resolved via FastAPI `Depends(...)` in `src/api/dependencies.py`.
>
> **100% Local Offline Execution Mandate via `docker-compose.yml`**: The entire system suite—including database (`postgres` with `pgvector`), message queue broker (`valkey`), FastAPI server (`server`), background scraper worker (`worker`), and black-box E2E test runner (`e2e`)—MUST be **100% executable locally out of the box via `docker compose up --build`** (or `make e2e`). When `OPENAI_API_KEY` is omitted, the system MUST seamlessly fall back to local deterministic embeddings and direct vector retrieval without crashing.

---

## 2. Pinned Directory Layout (Strict DDD Architecture)

```
searchthedocs/
├── Makefile                     # Root orchestrator: install, start, build, test, lint, format, e2e (REQUIRED)
├── README.md                    # Setup, architecture, API contract, and e2e instructions
├── docker-compose.yml           # postgres (pgvector) + valkey (queue) + server + worker + e2e (see §8)
├── .gitignore                   # __pycache__/, *.pyc, *.db, *.log, .venv/
├── pyproject.toml               # Poetry/uv/pip dependency definitions (Python 3.15)
├── src/
│   ├── __init__.py
│   ├── main.py                  # CLI entrypoint supporting 'server' and 'worker' subcommands
│   ├── config.py                # Environment variables & application settings
│   ├── domain/                  # CORE DOMAIN LAYER (Zero external I/O dependencies)
│   │   ├── __init__.py
│   │   ├── job.py               # IngestionJob entity & JobStatus enum
│   │   ├── chunk.py             # DocumentChunk entity & SearchQuery value object
│   │   ├── site.py              # IndexedSite entity
│   │   └── interfaces.py        # Abstract Domain Protocols (VectorRepository, Embedder, LLMClient, Crawler)
│   ├── application/             # APPLICATION LAYER (Use-Case Services & DI Injected Commands)
│   │   ├── __init__.py
│   │   ├── ingest_service.py    # IngestionAppService (coordinates crawling, chunking, embedding, repo)
│   │   └── search_service.py    # SearchAppService (coordinates vector search & RAG synthesis)
│   ├── infrastructure/          # INFRASTRUCTURE LAYER (Technology Adapters & Third-Party Integrations)
│   │   ├── __init__.py
│   │   ├── postgres/
│   │   │   ├── __init__.py
│   │   │   └── repository.py    # AsyncPG + pgvector HNSW repository adapter
│   │   ├── llm/
│   │   │   ├── __init__.py
│   │   │   └── openai_client.py # OpenAI-compatible API client (/v1/embeddings & /v1/chat/completions)
│   │   ├── queue/
│   │   │   ├── __init__.py
│   │   │   └── redis_queue.py   # Redis job queue producer & consumer adapter
│   │   └── scraper/
│   │       ├── __init__.py
│   │       ├── crawler.py       # Async HTTP crawler adapter (httpx + BeautifulSoup + lxml)
│   │       └── chunker.py       # Sliding window text chunker adapter
│   └── api/                     # PRIMARY ADAPTERS LAYER (FastAPI Web Framework & DI Container)
│       ├── __init__.py
│       ├── app.py               # FastAPI application factory & custom exception handlers
│       ├── dependencies.py      # Dependency Injection container / FastAPI Depends provider factories
│       └── routes.py            # REST API endpoints consuming injected application services
├── static/
│   ├── index.html               # Modern SPA search & indexing dashboard
│   ├── styles.css               # Modern CSS design system (dark/light theme, badges, progress bars)
│   └── app.js                   # Client JS for RAG search, live polling, and site selector
└── tests/
    ├── unit/
    │   ├── test_chunker.py      # Unit tests for text chunker
    │   └── test_vector.py       # Unit tests for domain entities & vector calculations
    ├── integration/
    │   └── test_crawler.py      # Integration tests against mock HTML pages
    └── e2e/
        ├── Dockerfile           # E2E test runner image (installs curl & jq)
        └── run_tests.sh         # Black-box job creation, scraping completion, and search assertions
```

---

## 3. Toolchain, Invocation, Exit Codes

### 3.1 Pinned Dependencies & Build System

#### `pyproject.toml`
```toml
[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[project]
name = "searchthedocs"
version = "0.1.0"
requires-python = ">=3.15"
dependencies = [
    "fastapi>=0.115.0",
    "uvicorn[standard]>=0.34.0",
    "httpx>=0.28.0",
    "beautifulsoup4>=4.12.0",
    "lxml>=5.3.0",
    "numpy>=2.1.0",
    "redis>=5.2.0",
    "pydantic>=2.10.0",
    "asyncpg>=0.30.0",
    "pgvector>=0.3.0",
    "jinja2>=3.1.0"
]

[project.optional-dependencies]
dev = [
    "pytest>=8.3.0",
    "pytest-asyncio>=0.25.0",
    "ruff>=0.8.0",
    "mypy>=1.14.0"
]
```

### 3.2 Root Makefile Targets (REQUIRED, defined exactly once)

- `make install` → `pip install -e .` (or `uv pip install -e .`).
- `make start` → Starts server: `python -m src.main server`.
- `make build` → Pre-compiles bytecode / validates imports (`python -m compileall src`).
- `make test` → Runs pytest suite (`pytest tests/unit tests/integration` — zero failures).
- `make lint` → Runs `ruff check .` AND `mypy --strict src` — zero findings.
- `make format` → `ruff format .` (idempotent).
- `make e2e` → `docker compose up --build --exit-code-from e2e` (host-run).

### 3.3 Environment Variables (Pinned Defaults)

- `PORT` (default `8080`) — HTTP API & Web UI listen port.
- `REDIS_URL` (default `redis://localhost:6379/0`).
- `POSTGRES_URL` (default `postgresql+asyncpg://postgres:postgres@localhost:5432/searchthedocs` locally, `postgresql+asyncpg://postgres:postgres@postgres:5432/searchthedocs` in Docker).
- `OPENAI_BASE_URL` (default `https://api.openai.com/v1`).
- `OPENAI_API_KEY` (default `""` — when empty, system uses local deterministic fallback).
- `LLM_MODEL` (default `gpt-4o-mini`).
- `EMBEDDING_MODEL` (default `text-embedding-3-small` or `tfidf`).
- `MAX_PAGES_PER_JOB` (default `50`).

---

## 4. REST API & Wire Contracts (Pinned, Black-Box)

### 4.1 Data Models & Request Schemas

#### IngestionJob Request Body (`POST /api/v1/jobs`)
```json
{
  "url": "https://docs.python.org/3/"
}
```

#### IngestionJob Response Model
```json
{
  "job_id": "job_12345678",
  "url": "https://docs.python.org/3/",
  "domain": "docs.python.org",
  "status": "COMPLETED",
  "pages_scraped": 15,
  "chunks_indexed": 120,
  "error": null,
  "created_at": "2026-08-03T12:00:00Z",
  "updated_at": "2026-08-03T12:01:30Z"
}
```
*Valid Statuses:* `PENDING`, `SCRAPING`, `INDEXING`, `COMPLETED`, `FAILED`.

#### IndexedSite Response Model (`GET /api/v1/sites`)
```json
[
  {
    "domain": "docs.python.org",
    "title": "Python 3 Documentation",
    "url": "https://docs.python.org/3/",
    "chunks_count": 120,
    "scraped_at": "2026-08-03T12:01:30Z"
  }
]
```

#### Search Result Response Model (`GET /api/v1/search?q=<query>&domain=<domain>`)
```json
{
  "query": "how to configure logging",
  "domain": "docs.python.org",
  "answer": "To configure logging in Python, call dictConfig() with a configuration dictionary containing options like formatters and handlers...",
  "results": [
    {
      "chunk_id": "chunk_99",
      "url": "https://docs.python.org/3/library/logging.config.html",
      "domain": "docs.python.org",
      "title": "logging.config — Logging configuration",
      "content": "To configure logging, call dictConfig() with a dictionary of configuration options...",
      "score": 0.885
    }
  ]
}
```
*Note on `answer` field:* `answer` is an `Optional[str]`. If `OPENAI_API_KEY` is provided, it contains the synthesized RAG response from the LLM. If `OPENAI_API_KEY` is empty or omitted, `answer` is `null`.

---

### 4.2 HTTP Endpoints

| Method | Path | Request Body | Success | Failure |
| :--- | :--- | :---: | :--- | :--- |
| `GET` | `/` | None | `200` HTML page | — |
| `POST` | `/api/v1/jobs` | `{"url": "string"}` | `202` `IngestionJob` | `400` `{"error":"invalid readthedocs URL"}` |
| `GET` | `/api/v1/jobs/{job_id}` | None | `200` `IngestionJob` | `404` `{"error":"job not found"}` |
| `GET` | `/api/v1/sites` | None | `200` array of `IndexedSite` | — |
| `GET` | `/api/v1/search?q=<query>&domain=<domain>` | Query params | `200` Search Result | `400` `{"error":"missing query parameter"}` |
| `GET` | `/healthz` | None | `200` `{"status":"ok"}` | — |

#### Verbatim Error Messages & Custom Exception Handler Requirement (MUST match exactly):
To comply with black-box E2E assertions, FastAPI MUST register custom exception handlers for `RequestValidationError` and `HTTPException` in `src/api/app.py`:
- Invalid URL input or malformed payload: `400 {"error":"invalid readthedocs URL"}`
- Job not found: `404 {"error":"job not found"}`
- Missing or empty search query parameter (`q` missing or `""`): `400 {"error":"missing query parameter"}`

---

## 5. Domain-Driven Design & SOLID Architecture

### 5.0 Core Domain Protocols (`src/domain/interfaces.py`)

All abstract behaviors MUST be declared as Python `typing.Protocol` interfaces in the domain layer. Infrastructure adapters implementation classes MUST adhere to these protocols:

```python
from typing import Protocol, Optional
from src.domain.job import IngestionJob
from src.domain.chunk import DocumentChunk
from src.domain.site import IndexedSite

class VectorRepositoryProtocol(Protocol):
    """Data access interface for job and chunk persistence."""
    async def create_job(self, job: IngestionJob) -> None: ...
    async def get_job(self, job_id: str) -> Optional[IngestionJob]: ...
    async def update_job(self, job: IngestionJob) -> None: ...
    async def get_sites(self) -> list[IndexedSite]: ...
    async def delete_domain_chunks(self, domain: str, exclude_job_id: str) -> None: ...
    async def upsert_chunks(self, chunks: list[DocumentChunk]) -> None: ...
    async def search_chunks(self, query_vector: list[float], domain: Optional[str], limit: int = 10) -> list[tuple[DocumentChunk, float]]: ...

class EmbedderProtocol(Protocol):
    """Interface for text vector embedding generation."""
    async def embed_text(self, text: str) -> list[float]: ...
    async def embed_batch(self, texts: list[str]) -> list[list[float]]: ...

class LLMClientProtocol(Protocol):
    """Interface for OpenAI-compatible RAG answer synthesis."""
    async def generate_rag_answer(self, query: str, context_chunks: list[DocumentChunk]) -> Optional[str]: ...

class CrawlerProtocol(Protocol):
    """Interface for web scraping."""
    async def crawl(self, start_url: str, max_pages: int) -> list[tuple[str, str, str]]: ...
```

---

### 5.1 Dependency Injection Container (`src/api/dependencies.py`)

All dependencies MUST be constructed and supplied using FastAPI's dependency injection (`Depends`):

```python
from fastapi import Depends
from src.domain.interfaces import VectorRepositoryProtocol, EmbedderProtocol, LLMClientProtocol
from src.infrastructure.postgres.repository import PostgresVectorRepository
from src.infrastructure.llm.openai_client import OpenAIClientAdapter
from src.application.search_service import SearchAppService

# Singleton / Request-scoped provider factories
def get_vector_repository() -> VectorRepositoryProtocol:
    return PostgresVectorRepository()

def get_embedder() -> EmbedderProtocol:
    return OpenAIClientAdapter()

def get_llm_client() -> LLMClientProtocol:
    return OpenAIClientAdapter()

def get_search_service(
    repo: VectorRepositoryProtocol = Depends(get_vector_repository),
    embedder: EmbedderProtocol = Depends(get_embedder),
    llm: LLMClientProtocol = Depends(get_llm_client),
) -> SearchAppService:
    # Explicit Constructor Injection
    return SearchAppService(repository=repo, embedder=embedder, llm_client=llm)
```

---

### 5.2 OpenAI-Compatible LLM Adapter (`src/infrastructure/llm/openai_client.py`)

The `OpenAIClientAdapter` implements `EmbedderProtocol` and `LLMClientProtocol`. When `OPENAI_API_KEY` is empty, it uses deterministic local hashing and returns `None` for synthesized answers.

```python
import os
import hashlib
import numpy as np
import httpx
from typing import Optional
from src.domain.chunk import DocumentChunk

class OpenAIClientAdapter:
    def __init__(self) -> None:
        self.base_url = os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1").rstrip("/")
        self.api_key = os.getenv("OPENAI_API_KEY", "")
        self.llm_model = os.getenv("LLM_MODEL", "gpt-4o-mini")
        self.dimension = 128

    async def embed_text(self, text: str) -> list[float]:
        if not self.api_key:
            # Deterministic Local Fallback (Offline Mode)
            digest = hashlib.md5(text[:512].encode("utf-8")).digest()
            vec = np.zeros(self.dimension, dtype=np.float32)
            words = text[:512].lower().split()
            for w in words:
                idx = int.from_bytes(hashlib.md5(w.encode()).digest()[:4], "big") % self.dimension
                vec[idx] += 1.0
            norm = np.linalg.norm(vec)
            return (vec / norm if norm > 0 else vec).tolist()

        async with httpx.AsyncClient(timeout=15.0) as client:
            resp = await client.post(
                f"{self.base_url}/embeddings",
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={"input": text[:512], "model": "text-embedding-3-small"}
            )
            data = resp.json()
            return data["data"][0]["embedding"]

    async def embed_batch(self, texts: list[str]) -> list[list[float]]:
        return [await self.embed_text(t) for t in texts]

    async def generate_rag_answer(self, query: str, context_chunks: list[DocumentChunk]) -> Optional[str]:
        if not self.api_key or not context_chunks:
            return None

        context_text = "\n\n".join([f"Source ({c.url}):\n{c.content}" for c in context_chunks[:5]])
        prompt = f"Documentation Context:\n{context_text}\n\nQuestion: {query}\n\nProvide a concise answer with source links:"

        async with httpx.AsyncClient(timeout=30.0) as client:
            resp = await client.post(
                f"{self.base_url}/chat/completions",
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={
                    "model": self.llm_model,
                    "messages": [
                        {"role": "system", "content": "You are a helpful documentation search assistant."},
                        {"role": "user", "content": prompt}
                    ],
                    "temperature": 0.2
                }
            )
            data = resp.json()
            return data["choices"][0]["message"]["content"]
```

---

### 5.3 Application Services (`src/application/`)

#### `SearchAppService` (`src/application/search_service.py`)
```python
from typing import Optional
from src.domain.interfaces import VectorRepositoryProtocol, EmbedderProtocol, LLMClientProtocol

class SearchAppService:
    def __init__(
        self,
        repository: VectorRepositoryProtocol,
        embedder: EmbedderProtocol,
        llm_client: LLMClientProtocol,
    ) -> None:
        self.repository = repository
        self.embedder = embedder
        self.llm_client = llm_client

    async def execute_search(self, query_str: str, domain: Optional[str]) -> dict:
        # 1. Embed query text
        query_vector = await self.embedder.embed_text(query_str)
        # 2. Vector search via repository
        matches = await self.repository.search_chunks(query_vector, domain=domain, limit=10)
        chunks = [m[0] for m in matches]
        # 3. Optional RAG answer synthesis
        answer = await self.llm_client.generate_rag_answer(query_str, chunks)
        
        return {
            "query": query_str,
            "domain": domain,
            "answer": answer,
            "results": [
                {
                    "chunk_id": c.chunk_id,
                    "url": c.url,
                    "domain": c.domain,
                    "title": c.title,
                    "content": c.content,
                    "score": round(score, 3)
                }
                for c, score in matches
            ]
        }
```

---

### 5.4 PostgreSQL + `pgvector` Schema & Repository Adapter (`src/infrastructure/postgres/repository.py`)

#### Database Schema:
```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS jobs (
    job_id        VARCHAR(64) PRIMARY KEY,
    url           TEXT NOT NULL,
    domain        VARCHAR(255) NOT NULL,
    status        VARCHAR(32) NOT NULL,
    pages_scraped INT DEFAULT 0,
    chunks_indexed INT DEFAULT 0,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS doc_chunks (
    chunk_id   VARCHAR(64) PRIMARY KEY,
    job_id     VARCHAR(64) REFERENCES jobs(job_id) ON DELETE CASCADE,
    domain     VARCHAR(255) NOT NULL,
    url        TEXT NOT NULL,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL,
    embedding  vector(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_doc_chunks_domain ON doc_chunks(domain);
CREATE INDEX IF NOT EXISTS idx_doc_chunks_embedding_hnsw 
ON doc_chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
```

#### Cosine Similarity Search Engine Query:
```sql
SELECT 
    chunk_id, url, domain, title, content, 
    1 - (embedding <=> $1::vector) AS score
FROM doc_chunks
WHERE ($2::text IS NULL OR $2 = '' OR $2 = 'all' OR domain = $2)
ORDER BY embedding <=> $1::vector
LIMIT 10;
```

---

## 6. Web User Interface UX & Component Specification (`static/`)

The single-page application (`static/index.html`, `static/styles.css`, `static/app.js`) is mounted at `/` by FastAPI:

```python
# src/api/app.py
from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse

app = FastAPI(title="searchthedocs")
app.mount("/static", StaticFiles(directory="static"), name="static")

@app.get("/")
def read_root():
    return FileResponse("static/index.html")
```

---

## 7. Implementation & Code Constraints

- **Single CLI Entrypoint (`src/main.py`):**
  - `python -m src.main server` $\rightarrow$ Launches Uvicorn + FastAPI HTTP server on `PORT`.
  - `python -m src.main worker` $\rightarrow$ Launches background Redis queue worker loop.
- **Horizontal Worker Scalability:** Worker processes are 100% stateless and atomic. Multiple worker containers can be deployed in parallel via `docker compose up --scale worker=N` to process job queues concurrently.
- **Strict Layer Decoupling:** `domain/` HAS ZERO imports from `infrastructure/` or `api/`.
- **File Limit:** No source file (`.py`) may exceed **500 lines** of code.

---

## 8. E2E Black-Box Harness (`docker-compose.yml`)

#### `Dockerfile`:
```dockerfile
FROM python:3.15-slim
WORKDIR /app
COPY pyproject.toml ./
RUN pip install --no-cache-dir .
COPY . .
EXPOSE 8080
CMD ["python", "-m", "src.main", "server"]
```

#### `tests/e2e/Dockerfile`:
```dockerfile
FROM python:3.15-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl jq && rm -rf /var/lib/apt/lists/*
COPY run_tests.sh /run_tests.sh
RUN chmod +x /run_tests.sh
ENTRYPOINT ["/bin/bash", "/run_tests.sh"]
```

#### `docker-compose.yml`:
```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: searchthedocs
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 2s
      timeout: 5s
      retries: 10

  valkey:
    image: valkey/valkey:8-alpine
    ports:
      - "6379:6379"

  server:
    build: .
    command: ["python", "-m", "src.main", "server"]
    environment:
      PORT: "8080"
      REDIS_URL: "redis://valkey:6379/0"
      POSTGRES_URL: "postgresql+asyncpg://postgres:postgres@postgres:5432/searchthedocs"
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
      valkey:
        condition: service_started

  worker:
    build: .
    command: ["python", "-m", "src.main", "worker"]
    environment:
      REDIS_URL: "redis://valkey:6379/0"
      POSTGRES_URL: "postgresql+asyncpg://postgres:postgres@postgres:5432/searchthedocs"
    depends_on:
      postgres:
        condition: service_healthy
      valkey:
        condition: service_started
      server:
        condition: service_started

  e2e:
    build: ./tests/e2e
    depends_on:
      - server
      - worker
    environment:
      API_URL: "http://server:8080"
```

#### `tests/e2e/run_tests.sh` Black-Box Assertions:
```bash
#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
echo "==> Testing Healthz endpoint..."
curl -sS --fail "${API_URL}/healthz" | grep '"status":"ok"'

echo "==> Submitting Ingestion Job..."
JOB_RESP=$(curl -sS -X POST "${API_URL}/api/v1/jobs" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://docs.python.org/3/"}')

JOB_ID=$(echo "$JOB_RESP" | jq -r '.job_id')
echo "==> Job created: ${JOB_ID}"

echo "==> Polling Job Completion..."
MAX_ATTEMPTS=30
ATTEMPT=0
STATUS=""
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  JOB_STATUS_RESP=$(curl -sS "${API_URL}/api/v1/jobs/${JOB_ID}")
  STATUS=$(echo "$JOB_STATUS_RESP" | jq -r '.status')
  echo "Attempt $((ATTEMPT+1)): status=${STATUS}"
  if [ "$STATUS" = "COMPLETED" ]; then
    break
  fi
  if [ "$STATUS" = "FAILED" ]; then
    echo "ERROR: Scraping job failed!"
    exit 1
  fi
  sleep 1
  ATTEMPT=$((ATTEMPT+1))
done

if [ "$STATUS" != "COMPLETED" ]; then
  echo "ERROR: Job timed out waiting for COMPLETED status"
  exit 1
fi

echo "==> Asserting Indexed Sites..."
curl -sS "${API_URL}/api/v1/sites" | jq -e '.[] | select(.domain=="docs.python.org")' > /dev/null

echo "==> Asserting Search Endpoint..."
SEARCH_RESP=$(curl -sS "${API_URL}/api/v1/search?q=logging&domain=docs.python.org")
echo "$SEARCH_RESP" | jq -e '.results | length > 0' > /dev/null

echo "==> All E2E assertions passed successfully!"
```

---

## 9. Documentation (`README.md`)

`README.md` must document: setup (`make install`), running locally (`make start`), DDD architecture layers, OpenAI-compatible LLM configuration, PostgreSQL + `pgvector` setup, REST API contract, Redis queue architecture, HNSW vector search implementation, UI features, and test execution (`make test`, `make lint`, `make e2e`).
