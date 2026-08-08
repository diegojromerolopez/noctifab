# searchthedocs Specification: Python ReadTheDocs RAG Search Engine with Worker Scraping Queue

## 1. Overview

`searchthedocs` is a RAG (Retrieval-Augmented Generation) documentation search platform written in **Python 3.15+ (FastAPI)**. It allows users to submit any ReadTheDocs URL (e.g. `https://<subdomain>.readthedocs.io/`), enqueues an asynchronous scraping job via **Redis / Valkey**, extracts documentation pages using background worker processes (`BeautifulSoup4` + `httpx`), indexes chunks into a vector database with vector embeddings (`sentence-transformers` / `numpy`), and serves semantic RAG search queries via a modern web UI and REST API.

The project tests key Dark Factory engineering seams:
- **Asynchronous Queue & Worker Seam:** Decoupled job submission (`POST /api/v1/jobs`) from background scraping workers.
- **RAG Vector Search & Chunking Seam:** Text extraction from HTML, sliding-window chunking, dense vector embedding generation, and domain-scoped cosine similarity vector retrieval.
- **Full-Stack Web Interface & API:** Embedded modern single-page search dashboard and OpenAPI REST endpoints.

> [!IMPORTANT]
> **Toolchain & Standards Mandate**: `searchthedocs` MUST be implemented in **Python 3.15+**. Code MUST pass `ruff check .` with ZERO warnings and strict `mypy --strict` compliance. Every source file MUST remain under 500 lines of code.
>
> **100% Local Execution Mandate via `docker-compose.yml`**: The entire system suite—including the message queue broker (`valkey`), FastAPI server (`server`), background scraper worker (`worker`), vector embeddings/database (`DATABASE_PATH`), and black-box E2E test runner (`e2e`)—MUST be **100% executable locally out of the box via `docker compose up --build`** (or `make e2e`). The application MUST NOT depend on external cloud services, remote vector APIs, or paid third-party credentials.
>
> **Mandatory E2E Black-Box Test Gate**: The repository MUST include a complete, automated End-to-End (E2E) test suite (`tests/e2e/Dockerfile` + `tests/e2e/run_tests.sh`) executable via `make e2e` or `docker compose up --build --exit-code-from e2e`. The E2E suite MUST perform black-box HTTP assertions against the live containerized application (`${API_URL}`), validating health check (`GET /healthz`), async job submission (`POST /api/v1/jobs`), scraping worker completion polling (`GET /api/v1/jobs/{job_id}`), site directory listing (`GET /api/v1/sites`), vector search query retrieval (`GET /api/v1/search`), and exit `0` on total success or `1` on failure.

---

## 2. Pinned Directory Layout

```
searchthedocs/
├── Makefile                     # Root orchestrator: install, start, build, test, lint, format, e2e (REQUIRED)
├── README.md                    # Setup, architecture, API contract, and e2e instructions
├── docker-compose.yml           # valkey (queue/cache) + server + worker + e2e test runner (see §8)
├── .gitignore                   # __pycache__/, *.pyc, *.db, *.log, .venv/
├── pyproject.toml               # Poetry/uv/pip dependency definitions (Python 3.15)
├── src/
│   ├── __init__.py
│   ├── main.py                  # CLI entrypoint supporting 'server' and 'worker' subcommands
│   ├── config.py                # Environment variables & settings
│   ├── api/
│   │   ├── __init__.py
│   │   ├── app.py               # FastAPI application factory & static file mounting
│   │   └── routes.py            # API routes (/api/v1/jobs, /api/v1/search, /api/v1/sites, /healthz, /)
│   ├── scraper/
│   │   ├── __init__.py
│   │   ├── crawler.py           # HTML crawler & link extractor (httpx + BeautifulSoup)
│   │   └── chunker.py           # Text chunking logic with window overlap
│   ├── vector/
│   │   ├── __init__.py
│   │   ├── store.py             # SQLite WAL-mode vector store & domain-filtered cosine search engine
│   │   └── embedder.py          # SentenceTransformers / TF-IDF embedding generator
│   └── queue/
│       ├── __init__.py
│       ├── redis_queue.py       # Redis list job producer and worker consumer loop
│       └── job.py               # IngestionJob Pydantic schema & state machine
├── static/
│   ├── index.html               # Modern SPA search & indexing dashboard
│   ├── styles.css               # Modern CSS design system (dark/light theme, badges, progress bars)
│   └── app.js                   # Client JS for RAG search, live polling, and site selector
└── tests/
    ├── unit/
    │   ├── test_chunker.py      # Unit tests for text chunker
    │   └── test_vector.py       # Unit tests for vector store & cosine similarity
    ├── integration/
    │   └── test_crawler.py      # Integration tests against mock HTML pages
    └── e2e/
        ├── Dockerfile           # E2E test runner image
        └── run_tests.sh         # Black-box job creation, scraping completion, and search assertions
```

---

## 3. Toolchain, Invocation, Exit Codes

### 3.1 Pinned Dependencies & Scripts

#### `pyproject.toml`
- **Runtime Dependencies:** `fastapi ^0.115`, `uvicorn ^0.34`, `httpx ^0.28`, `beautifulsoup4 ^4.12`, `sentence-transformers ^3.3` (or `numpy ^2.1` + `scikit-learn ^1.6`), `redis ^5.2`, `pydantic ^2.10`, `jinja2 ^3.1`.
- **Dev Dependencies:** `pytest ^8.3`, `pytest-asyncio ^0.25`, `ruff ^0.8`, `mypy ^1.14`.

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
- `DATABASE_PATH` (default `./searchthedocs.db`).
- `EMBEDDING_MODEL` (default `all-MiniLM-L6-v2` or `tfidf`).
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

#### IndexedSite Response Model
```json
{
  "domain": "docs.python.org",
  "title": "Python 3 Documentation",
  "url": "https://docs.python.org/3/",
  "chunks_count": 120,
  "scraped_at": "2026-08-03T12:01:30Z"
}
```

#### Search Result Response Model (`GET /api/v1/search?q=<query>&domain=<domain>`)
```json
{
  "query": "how to configure logging",
  "domain": "docs.python.org",
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

#### Verbatim Error Messages (MUST match exactly):
- Invalid URL input: `400 {"error":"invalid readthedocs URL"}`
- Job not found: `404 {"error":"job not found"}`
- Missing search query parameter (`q` missing or empty): `400 {"error":"missing query parameter"}`

---

## 5. Architecture & Explicit Edge Case Safeguards

### 5.1 Architecture & Component Flow

```
  ┌────────────────────────────────────────────────────────┐
  │ User Browser / Single-Page Application (static/)       │
  └───────────┬───────────────────────────────▲────────────┘
              │ 1. POST /api/v1/jobs          │ 4. GET /api/v1/search
              ▼                               │
  ┌───────────────────────────────────────────┴────────────┐
  │ FastAPI Application Server (src/api/)                  │
  └───────────┬───────────────────────────────▲────────────┘
              │ 2. Enqueue Job                │ 5. Cosine Search
              ▼                               │
   ┌────────────────────┐          ┌──────────┴────────────┐
   │ Redis Job Queue    │          │ SQLite Vector Database│
   └──────────┬─────────┘          └──────────▲────────────┘
              │ 3. Dequeue                    │ Write Chunks &
              ▼                               │ Vectors
   ┌──────────────────────────────────────────┴────────────┐
   │ Background Worker Process (src/main.py worker)        │
   │ - Crawls HTML (httpx + BeautifulSoup4)               │
   │ - Chunks Text & Computes Embeddings (numpy / ST)      │
   └───────────────────────────────────────────────────────┘
```

---

### 5.2 Domain Parsing & URL Validation Safeguards

When receiving `POST /api/v1/jobs`, the system MUST validate and normalize the target domain:

```python
from urllib.parse import urlparse

def extract_domain(url_str: str) -> str:
    """Extracts and normalizes hostname from URL."""
    url_str = url_str.strip()
    if not url_str.startswith(("http://", "https://")):
        url_str = "https://" + url_str
    parsed = urlparse(url_str)
    if not parsed.netloc:
        raise ValueError("Invalid URL: missing domain")
    # Return hostname (e.g. 'docs.python.org')
    return parsed.netloc.lower()
```

- Invalid format (e.g. `""`, `"not-a-url"`, `"://bad"`) $\rightarrow$ raises `ValueError`, API returns HTTP `400 {"error":"invalid readthedocs URL"}`.

---

### 5.3 Crawling Guardrails: Depth, Domain Restricting & Deduplication

To prevent infinite crawl loops, worker processes MUST enforce three strict guardrails:

```python
from urllib.parse import urljoin, urlparse
from bs4 import BeautifulSoup

def crawl_site(start_url: str, max_pages: int = 50) -> list[tuple[str, str, str]]:
    """
    Crawls pages bounded strictly to the target domain up to max_pages limit.
    Returns list of (page_url, page_title, page_text_content).
    """
    target_domain = urlparse(start_url).netloc.lower()
    visited: set[str] = set()
    queue: list[str] = [start_url]
    results: list[tuple[str, str, str]] = []
    
    while queue and len(results) < max_pages:
        current_url = queue.pop(0)
        clean_url = current_url.split("#")[0]
        if clean_url in visited:
            continue
        visited.add(clean_url)
        
        # Fetch page HTML (handle errors gracefully)
        try:
            # ... httpx.get(clean_url, timeout=5.0) ...
            title, text, links = extract_page_data(html_content, clean_url)
            if text:
                results.append((clean_url, title, text))
                
            for link in links:
                parsed_link = urlparse(link)
                # Strict Domain Restricting Guard
                if parsed_link.netloc.lower() == target_domain and link not in visited:
                    queue.append(link)
        except Exception:
            continue
            
    return results
```

---

### 5.4 Vector Embedding Generation & Fallback Embedder (`src/vector/embedder.py`)

To prevent heavy PyTorch model download timeouts in container environments, the embedder system includes an explicit TF-IDF / Hash fallback:

```python
import json
import numpy as np

class VectorEmbedder:
    """Generates normalized float32 vector embeddings for text chunks."""
    def __init__(self, model_name: str = "tfidf"):
        self.dimension = 128
        self.model_name = model_name
        
    def embed_text(self, text: str) -> list[float]:
        """Returns a 128-dimensional normalized float list."""
        # Simple deterministic character n-gram hashing vectorizer (0-dependency fallback)
        vec = np.zeros(self.dimension, dtype=np.float32)
        words = text.lower().split()
        for word in words:
            idx = hash(word) % self.dimension
            vec[idx] += 1.0
        norm = np.linalg.norm(vec)
        if norm > 0:
            vec = vec / norm
        return vec.tolist()
```

---

### 5.5 SQLite WAL-Mode Concurrency & Vector Serialization (`src/vector/store.py`)

To prevent `sqlite3.OperationalError: database is locked` when the API server and worker process write concurrently, SQLite database connections MUST enable **Write-Ahead Logging (WAL)**:

```python
import sqlite3
import json

def get_db_connection(db_path: str) -> sqlite3.Connection:
    conn = sqlite3.connect(db_path, timeout=30.0)
    conn.row_factory = sqlite3.Row
    # Enable WAL mode for high-concurrency multi-process access
    conn.execute("PRAGMA journal_mode=WAL;")
    conn.execute("PRAGMA synchronous=NORMAL;")
    return conn

def serialize_embedding(vec: list[float]) -> bytes:
    """Serializes vector float list to UTF-8 JSON bytes for SQLite BLOB storage."""
    return json.dumps(vec).encode("utf-8")

def deserialize_embedding(blob: bytes) -> list[float]:
    """Deserializes SQLite BLOB back to float vector list."""
    return json.loads(blob.decode("utf-8"))
```

#### Database Schema:
```sql
CREATE TABLE IF NOT EXISTS jobs (
    job_id        TEXT PRIMARY KEY,
    url           TEXT NOT NULL,
    domain        TEXT NOT NULL,
    status        TEXT NOT NULL,
    pages_scraped INTEGER DEFAULT 0,
    chunks_indexed INTEGER DEFAULT 0,
    error         TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS doc_chunks (
    chunk_id   TEXT PRIMARY KEY,
    job_id     TEXT NOT NULL,
    domain     TEXT NOT NULL,
    url        TEXT NOT NULL,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL,
    embedding  BLOB NOT NULL, -- UTF-8 JSON encoded list[float]
    created_at TEXT NOT NULL,
    FOREIGN KEY(job_id) REFERENCES jobs(job_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_doc_chunks_domain ON doc_chunks(domain);
```

---

## 6. Web User Interface UX & Component Specification (`static/`)

The single-page application (`static/index.html`, `static/styles.css`, `static/app.js`) is mounted at `/` by FastAPI:

```python
# src/api/app.py
from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse

app = FastAPI(title="searchreadthedocs")
app.mount("/static", StaticFiles(directory="static"), name="static")

@app.get("/")
def read_root():
    return FileResponse("static/index.html")
```

### 6.1 UI Component Architecture

1. **Header Bar (`Header`):** Shows system status badge (`Online` / `Connecting...`) and live count of indexed sites (`GET /api/v1/sites`).
2. **Ingest Dashboard (`IngestPanel`):** URL input field, `Start Indexing` button, and progress card polling `GET /api/v1/jobs/{job_id}` every 1 second displaying status pill, `Pages Scraped`, and `Chunks Indexed`.
3. **Domain Filter & Site Selector (`SiteSelector`):** Dropdown select dynamically populated from `GET /api/v1/sites`. Allows selecting "All Domains" or a specific domain (e.g. `docs.python.org`).
4. **RAG Search Feed (`SearchFeed`):** Renders result cards with clickable page title links (`target="_blank"`), breadcrumbs, content excerpts, and relevance match score pills (e.g. `88.5% Match`).

---

## 7. Implementation & Code Constraints

- **Single CLI Entrypoint (`src/main.py`):**
  - `python -m src.main server` $\rightarrow$ Launches Uvicorn + FastAPI HTTP server on `PORT`.
  - `python -m src.main worker` $\rightarrow$ Launches background Redis queue worker loop.
- **Clean Architecture:** `src/scraper/`, `src/vector/`, and `src/queue/` are decoupled.
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

#### `docker-compose.yml`:
```yaml
services:
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
      DATABASE_PATH: "/tmp/searchreadthedocs.db"
    ports:
      - "8080:8080"
    depends_on:
      - valkey

  worker:
    build: .
    command: ["python", "-m", "src.main", "worker"]
    environment:
      REDIS_URL: "redis://valkey:6379/0"
      DATABASE_PATH: "/tmp/searchreadthedocs.db"
    depends_on:
      - valkey
      - server

  e2e:
    build: ./tests/e2e
    depends_on:
      - server
      - worker
    environment:
      API_URL: "http://server:8080"
```

#### `tests/e2e/run_tests.sh` Black-Box Assertions:
1. `GET ${API_URL}/healthz` returns `200 {"status":"ok"}`.
2. `POST ${API_URL}/api/v1/jobs` with `{"url": "https://docs.python.org/3/"}` returns `202` and a `job_id`.
3. Polls `GET ${API_URL}/api/v1/jobs/{job_id}` until status is `COMPLETED`.
4. `GET ${API_URL}/api/v1/sites` returns array containing `docs.python.org`.
5. `GET ${API_URL}/api/v1/search?q=logging` returns `200` with non-empty results array and positive score values.
6. Exits `0` on success or `1` on failure.

---

## 9. Documentation (`README.md`)

`README.md` must document: setup (`make install`), running locally (`make start`), REST API contract, Redis queue architecture, vector search implementation, UI features, and test execution (`make test`, `make lint`, `make e2e`).
