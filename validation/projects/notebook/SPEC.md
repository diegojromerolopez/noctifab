# notebook Specification: Full-Stack React + Fastify Notes App with Auth & WebSockets

## 1. Overview

`notebook` is a full-stack monorepo application for managing personal notes, featuring a **TypeScript (strict)** backend on **Fastify (v5)** persisting to **PostgreSQL**, user authentication (JWT + bcrypt), real-time push updates via **WebSockets**, and an interactive single-page frontend built with **React** and **Vite**.

The project is structured into two explicit subfolders:
- `backend/`: Fastify REST API, JWT authentication middleware, WebSocket server (`/ws`), PostgreSQL connection pool, DDL migrations, repository-layer SQL, and unit/integration test suites.
- `frontend/`: React single-page application (SPA) with user registration/login views, split-pane notes workspace, real-time WebSocket state synchronization, and component unit tests.

> [!IMPORTANT]
> **PostgreSQL Mandate**: `notebook` MUST strictly use **PostgreSQL 16** (via the standard `pg` driver `pg.Pool`) as its relational database engine for persistence. SQLite, MySQL, or fallback in-memory stores in production code are strictly forbidden. All SQL queries, migrations, indexes, and schema definitions MUST use PostgreSQL syntax and native types (`BIGSERIAL`, `TIMESTAMPTZ`, `now()`).
>
> **ID Precision Requirement**: PostgreSQL `BIGSERIAL` columns are returned by the `pg` driver as strings by default. The backend MUST parse `id` columns to JavaScript `number` (e.g. using `parseInt(row.id, 10)` or `pg.types.setTypeParser(20, val => parseInt(val, 10))`) so JSON payloads return `"id": 1` (number), NOT `"id": "1"` (string).

---

## 2. Pinned Directory Layout

```
notebook/
├── Makefile                     # Root orchestrator: install, start, build, test, lint, format, e2e (REQUIRED)
├── README.md                    # Setup, architecture, API contract, and e2e instructions
├── docker-compose.yml           # db (Postgres) + backend + frontend + e2e test runner (see §9)
├── .gitignore                   # node_modules/, dist/, build/, *.log
├── backend/
│   ├── Dockerfile               # node:22-alpine container definition for backend (see §9.3)
│   ├── package.json             # Backend dependencies & scripts (see §3.1)
│   ├── tsconfig.json            # "strict": true, NodeNext target
│   ├── eslint.config.mjs        # typescript-eslint recommended
│   ├── .prettierrc.json
│   ├── migrations/
│   │   ├── 001_users.sql        # users table + bookkeeping (see §6)
│   │   └── 002_notes.sql        # notes table with user_id foreign key (see §6)
│   ├── src/
│   │   ├── index.ts             # Server entrypoint: bootstrap, listen on PORT (default 3000)
│   │   ├── app.ts               # buildApp(userRepo, noteRepo, wsHub, config): Fastify factory (DI)
│   │   ├── db.ts                # createPool(databaseUrl): pg Pool factory + type parsers
│   │   ├── migrate.ts           # Automatic DDL migration runner
│   │   ├── repos/
│   │   │   ├── user-repo.ts     # UserRepository: user SQL operations
│   │   │   └── notes-repo.ts    # NoteRepository: user-scoped note SQL operations
│   │   ├── routes/
│   │   │   ├── auth.ts          # POST /api/v1/auth/register, POST /api/v1/auth/login, GET /api/v1/auth/me
│   │   │   ├── notes.ts         # Note CRUD routes (/api/v1/notes)
│   │   │   └── ws.ts            # WebSocket handler (/ws?token=<jwt>)
│   │   ├── services/
│   │   │   ├── auth-service.ts  # Password hashing (bcrypt) & JWT token sign/verify
│   │   │   └── ws-hub.ts        # Client connection hub & event broadcaster
│   │   └── types.ts             # Domain & DTO types (User, Note, WSEvent)
│   └── tests/
│       ├── unit/
│       │   ├── auth-service.test.ts # Unit tests for auth & password hashing
│       │   └── notes-repo.test.ts   # Repository unit tests against stub pool
│       └── integration/
│           ├── helpers.ts       # Ephemeral PG cluster start/stop (see §9.2)
│           ├── auth.test.ts     # Real Postgres: user registration & login round trip
│           └── crud.test.ts     # Real Postgres: authenticated CRUD & WebSocket event emissions
├── frontend/
│   ├── Dockerfile               # node:22-alpine container definition for frontend (see §9.3)
│   ├── package.json             # Frontend dependencies & scripts (see §3.1)
│   ├── tsconfig.json            # React & Vite TypeScript configuration
│   ├── eslint.config.mjs
│   ├── vite.config.ts           # Vite build & proxy config
│   ├── index.html               # SPA HTML entry point
│   ├── src/
│   │   ├── main.tsx             # React DOM root render
│   │   ├── App.tsx              # Root component & auth route guard
│   │   ├── components/
│   │   │   ├── AuthForm.tsx     # Combined Login / Register tabbed view
│   │   │   ├── Header.tsx       # Navigation header, user email, logout, WS status badge
│   │   │   ├── NoteList.tsx     # Filterable sidebar list of user notes
│   │   │   └── NoteEditor.tsx   # Active note title/content editor & viewer
│   │   ├── context/
│   │   │   ├── AuthContext.tsx  # Global JWT token state & login/logout actions
│   │   │   └── NotesContext.tsx # Notes state manager & WebSocket subscription loop
│   │   ├── services/
│   │   │   ├── api.ts           # Typed fetch wrapper with Bearer token header
│   │   │   └── ws.ts            # Auto-reconnecting WebSocket client logic
│   │   └── types.ts             # Frontend Note, User, and WSEvent types
│   └── tests/
│       └── components.test.tsx  # React component unit tests (Vitest + Testing Library)
└── tests/
    └── e2e/
        ├── Dockerfile           # Test runner image (node:22-alpine + curl + wscat)
        └── run_tests.sh         # Black-box HTTP auth, REST CRUD, WebSocket & SPA assertions
```

---

## 3. Toolchain, Invocation, Exit Codes

### 3.1 Pinned Dependencies & Scripts

#### `backend/package.json`
- **Runtime dependencies:** `fastify ^5.2.0`, `@fastify/jwt ^9.0.0`, `@fastify/websocket ^11.0.0`, `@fastify/cors ^10.0.0`, `pg ^8.13.0`, `bcryptjs ^2.4.3`.
- **Dev dependencies:** `typescript ^5.7.0`, `tsx ^4.19.0`, `@types/node ^22.10.0`, `@types/pg ^8.11.0`, `@types/bcryptjs ^2.4.0`, `vitest ^3.0.0`, `@vitest/coverage-v8 ^3.0.0`, `eslint ^9.17.0`, `typescript-eslint ^8.18.0`, `prettier ^3.4.0`.
- **Scripts:**
  - `npm run start` → `tsx src/index.ts`
  - `npm run build` → `tsc -p tsconfig.json`
  - `npm run test` → `vitest run`
  - `npm run lint` → `eslint src tests` AND `npx tsc --noEmit`

#### `frontend/package.json`
- **Runtime dependencies:** `react ^18.3.0`, `react-dom ^18.3.0`, `serve ^14.2.0`.
- **Dev dependencies:** `vite ^6.0.0`, `@vitejs/plugin-react ^4.3.0`, `typescript ^5.7.0`, `vitest ^3.0.0`, `@testing-library/react ^16.1.0`, `@testing-library/jest-dom ^6.6.0`, `jsdom ^26.0.0`, `eslint ^9.17.0`, `typescript-eslint ^8.18.0`, `prettier ^3.4.0`.
- **Scripts:**
  - `npm run dev` → `vite`
  - `npm run build` → `vite build` (outputs static bundle to `frontend/dist`)
  - `npm run preview` → `serve -s dist -l 5173`
  - `npm run test` → `vitest run`
  - `npm run lint` → `eslint src` AND `npx tsc --noEmit`

### 3.2 Root Makefile Targets (REQUIRED, defined exactly once)

The root `Makefile` orchestrates both subfolders:

- `make install` → Runs `npm install` in `backend/` AND `frontend/`.
- `make start` → Starts `backend` server (port 3000) and serves `frontend` (`npm run preview` or `vite` on port 5173).
- `make build` → Compiles backend (`npm run build` in `backend/`) and frontend (`npm run build` in `frontend/`).
- `make test` → Executes `npm run test` in both `backend/` and `frontend/` (zero failures).
- `make lint` → Executes `npm run lint` in both `backend/` and `frontend/` — zero findings.
- `make format` → Runs prettier across `backend/` and `frontend/` (idempotent).
- `make e2e` → Executes `docker compose up --build --exit-code-from e2e` (host-run).

### 3.3 Configuration Environment Variables (Pinned Defaults)

- `PORT` (default `3000`) — Backend HTTP & WebSocket listen port.
- `DATABASE_URL` (default `postgres://postgres:postgres@localhost:5432/notebook`).
- `JWT_SECRET` (default `notebook-super-secret-jwt-key-change-in-production`).
- `VITE_API_URL` (default `http://localhost:3000`).
- `VITE_WS_URL` (default `ws://localhost:3000`).

### 3.4 Exit Codes
- Clean `SIGINT`/`SIGTERM` shutdown → exit `0`.
- Startup failure (DB connection failure, migration failure, invalid port) → log `notebook: <reason>` to stderr, exit `1`.

---

## 4. Authentication & REST API Contracts (Pinned, Black-Box)

All API responses use JSON bodies. Timestamps are PostgreSQL `TIMESTAMPTZ` ISO-8601 UTC strings with millisecond precision (e.g. `2026-08-03T12:00:00.000Z`).

### 4.1 Data Models & Shapes

#### User Object
```json
{
  "id": 1,
  "email": "user@example.com",
  "created_at": "2026-08-03T12:00:00.000Z"
}
```

#### Note Object (Scoped to User)
```json
{
  "id": 10,
  "user_id": 1,
  "title": "Project Meeting Notes",
  "content": "Discussed architecture and release dates.",
  "created_at": "2026-08-03T12:05:00.000Z",
  "updated_at": "2026-08-03T12:10:00.000Z"
}
```

---

### 4.2 Auth Endpoints (`/api/v1/auth`)

| Method | Path | Auth Required | Success | Failure |
| :--- | :--- | :---: | :--- | :--- |
| `POST` | `/api/v1/auth/register` | No | `201` `{"token": "<jwt>", "user": User}` | `400` `{"error":"email already registered"}` / `{"error":"validation failed"}` |
| `POST` | `/api/v1/auth/login` | No | `200` `{"token": "<jwt>", "user": User}` | `401` `{"error":"invalid credentials"}` |
| `GET` | `/api/v1/auth/me` | Yes (`Bearer`) | `200` `{"user": User}` | `401` `{"error":"unauthorized"}` |

#### Exact Error Strings (MUST match verbatim):
- Email already registered: `400 {"error":"email already registered"}`
- Validation failed (missing/malformed fields): `400 {"error":"validation failed"}`
- Invalid login password/email: `401 {"error":"invalid credentials"}`
- Missing/invalid JWT token: `401 {"error":"unauthorized"}`

#### Auth Payload Rules:
- `email`: Required, valid email string, lowercased prior to storage.
- `password`: Required, minimum 6 characters. Passwords MUST be stored hashed using `bcrypt` (salt rounds ≥ 10).

---

### 4.3 Authenticated Notes API (`/api/v1/notes`)

All `/api/v1/notes` endpoints require an `Authorization: Bearer <jwt>` HTTP header. Operations strictly filter and operate on notes owned by the authenticated `user_id`. Attempting to access, edit, or delete another user's note returns `404 {"error":"note not found"}`.

| Method | Path | Success | Failure |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/notes` | `200` array of Note objects | `401` `{"error":"unauthorized"}` |
| `GET` | `/api/v1/notes?query=<q>` | `200` filtered array | `401` `{"error":"unauthorized"}` |
| `GET` | `/api/v1/notes/:id` | `200` Note object | `401` / `404` `{"error":"note not found"}` |
| `POST` | `/api/v1/notes` | `201` created Note object | `400` `{"error":"validation failed"}` / `401` |
| `PUT` | `/api/v1/notes/:id` | `200` updated Note object | `400` / `401` / `404` |
| `DELETE` | `/api/v1/notes/:id` | `204` empty body | `401` / `404` |
| `GET` | `/healthz` | `200` `{"status":"ok"}` (Public) | — |

#### Request Validation & Filtering Rules:
- `POST /api/v1/notes`: Body `{"title": string, "content"?: string}`. `title` required, non-empty, ≤ 200 chars; `content` optional (defaults to `""`), ≤ 10,000 chars. Validation failure $\rightarrow$ `400 {"error":"validation failed"}`.
- `PUT /api/v1/notes/:id`: Body `{"title"?, "content"?}` — at least one field required, same constraints. Updating a note bumps `updated_at` to `now()`. Validation failure $\rightarrow$ `400 {"error":"validation failed"}`.
- `GET /api/v1/notes`: Sorted by `updated_at` DESC, then `id` DESC.
- `?query=<q>`: Case-insensitive search matching `title` OR `content` (ILIKE `%<q>%`). `q` is URL-decoded.

---

## 5. WebSocket Specification (`/ws`)

The backend exposes a WebSocket endpoint at `/ws` for pushing real-time updates when notes are created, modified, or deleted.

### 5.1 Connection & Authentication Handshake
- WebSocket URL: `ws://<host>:<port>/ws?token=<jwt>`
- Authentication: When a WebSocket client connects, the server extracts `token` from the query parameters (`req.query.token`). The server verifies this token using `app.jwt.verify(token)`.
- Rejection: If `token` is missing, expired, or invalid, the server MUST close the connection immediately (close code `4001` or HTTP `401`).

### 5.2 Server-to-Client Event Broadcast
When an authenticated user mutates a note via REST API (`POST`, `PUT`, `DELETE`), the backend broadcasts a JSON event over all active WebSocket connections belonging to that `user_id`.

#### Broadcast Event Shapes:

1. **Note Created Event:**
```json
{
  "type": "NOTE_CREATED",
  "payload": {
    "id": 12,
    "user_id": 1,
    "title": "New Idea",
    "content": "Draft content...",
    "created_at": "2026-08-03T12:15:00.000Z",
    "updated_at": "2026-08-03T12:15:00.000Z"
  }
}
```

2. **Note Updated Event:**
```json
{
  "type": "NOTE_UPDATED",
  "payload": {
    "id": 12,
    "user_id": 1,
    "title": "Updated Idea Title",
    "content": "Revised draft content...",
    "created_at": "2026-08-03T12:15:00.000Z",
    "updated_at": "2026-08-03T12:20:00.000Z"
  }
}
```

3. **Note Deleted Event:**
```json
{
  "type": "NOTE_DELETED",
  "payload": {
    "id": 12
  }
}
```

---

## 6. Database Schema & Migrations (`backend/migrations/`)

`backend/migrations/001_users.sql`:
```sql
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`backend/migrations/002_notes.sql`:
```sql
CREATE TABLE IF NOT EXISTS notes (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notes_user_updated ON notes(user_id, updated_at DESC);
```

- Migrations apply automatically at backend startup (`backend/src/migrate.ts`).
- `schema_migrations` ensures each script executes exactly once.

---

## 7. Frontend Architecture & React User Experience (`frontend/`)

The frontend is an interactive React single-page application designed for effortless note management with zero manual refreshes required.

### 7.1 Component Breakdown & Workspace Layout

1. **Unauthenticated View (`AuthForm.tsx`):**
   - Tabbed or toggling card with **Login** and **Register** modes.
   - Input fields for `Email` and `Password` with inline client-side validation error messages.
   - Stores returned JWT token in `localStorage` and updates `AuthContext`.

2. **Authenticated View (`App.tsx` layout):**
   - **`Header.tsx`:** Displays app logo, current user email, WebSocket connection status badge (`Connected` green / `Reconnecting` yellow / `Offline` red), and a `Logout` button.
   - **Sidebar (`NoteList.tsx`):**
     - Top bar with `New Note` button and a live search filter input field (`query`).
     - Scrollable list of user notes showing note `title`, formatted `updated_at` relative time, and a snippet of `content`.
     - Highlights the currently active note.
   - **Main Editor Pane (`NoteEditor.tsx`):**
     - Title input field and rich content textarea.
     - Auto-saves changes or provides explicit `Save` / `Delete` actions.
     - Displays `Created at` and `Last modified` timestamps.

3. **Real-Time WebSocket Synchronization (`NotesContext.tsx` + `ws.ts`):**
   - Automatically establishes a WebSocket connection to `ws://<host>:<port>/ws?token=<jwt>` upon user login.
   - Listens for incoming `NOTE_CREATED`, `NOTE_UPDATED`, and `NOTE_DELETED` events.
   - Dynamically updates the local React `notes` state array:
     - On `NOTE_CREATED`: Prepends the new note to the list.
     - On `NOTE_UPDATED`: Updates the matching note in place (updating list title, snippet, and current open editor content if currently active).
     - On `NOTE_DELETED`: Removes the note from the list and clears active editor if currently selected.
   - Implements automatic reconnection exponential backoff if the socket disconnects.

---

## 8. Implementation & Software Engineering Constraints

- **TypeScript Strict Mode:** Both `backend/tsconfig.json` and `frontend/tsconfig.json` set `"strict": true`. `npx tsc --noEmit` must pass with zero errors in both subfolders.
- **CORS Configuration:** `backend/src/app.ts` MUST register `@fastify/cors` allowing origins from `VITE_API_URL` and `http://localhost:5173`.
- **SOLID & DDD Principles:**
  - `UserRepository` and `NoteRepository` abstract SQL interactions behind clean domain interfaces.
  - Route handlers handle request parsing and status code mapping only.
  - `ws-hub.ts` encapsulates connection management and broadcast logic.
- **Dependency Injection:** `buildApp(userRepo, noteRepo, wsHub, config)` is a factory function. Database pools and repositories are injected; no global singletons.
- **File Size Limit:** No single source file (`.ts`, `.tsx`) may exceed **500 lines** of code.

---

## 9. Testing Requirements & E2E Black-Box Harness

### 9.1 Unit & Integration Testing
1. **Backend Unit (`backend/tests/unit/`):** Tests auth password hashing, token validation, and repository SQL logic using stubbed pools.
2. **Backend Integration (`backend/tests/integration/`):** Executes against a REAL ephemeral PostgreSQL instance (see §9.2). Verifies migrations, user registration, JWT login, note CRUD, and WebSocket frame emissions.
3. **Frontend Component Unit (`frontend/tests/`):** Tests rendering of `AuthForm`, `NoteList`, and `NoteEditor` under synthetic context state using Vitest + Testing Library.

### 9.2 Ephemeral PostgreSQL Integration Test Setup
The validation container runs as `root`; PostgreSQL refuses to run as root, so integration tests MUST start the DB as a dedicated OS user. Test helpers (`backend/tests/integration/helpers.ts`) must, before the test suite:

1. Create/own a temp data dir writable by the `postgres` OS user:
   ```bash
   mkdir -p /tmp/notebook-pg
   chown -R postgres:postgres /tmp/notebook-pg
   ```
2. Initialize and start a PostgreSQL cluster as the `postgres` user:
   ```bash
   su -s /bin/sh postgres -c 'initdb -D /tmp/notebook-pg/data --no-locale -E UTF8'
   su -s /bin/sh postgres -c 'pg_ctl -D /tmp/notebook-pg/data -l /tmp/notebook-pg/log -o "-p 55432 -k /tmp/notebook-pg" start'
   ```
3. Create the test database: `su -s /bin/sh postgres -c 'createdb -h /tmp/notebook-pg -p 55432 notebook'`
4. Set `DATABASE_URL=postgres://postgres@localhost:55432/notebook` and run tests.
5. Stop the cluster in an `afterAll` hook (`su -s /bin/sh postgres -c 'pg_ctl -D /tmp/notebook-pg/data stop'`).

---

### 9.3 E2E Black-Box Harness (`docker-compose.yml`)

#### `backend/Dockerfile`:
```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install --no-audit --no-fund
COPY . .
RUN npm run build
EXPOSE 3000
CMD ["npm", "run", "start"]
```

#### `frontend/Dockerfile`:
```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install --no-audit --no-fund
COPY . .
RUN npm run build
EXPOSE 5173
CMD ["npx", "serve", "-s", "dist", "-l", "5173"]
```

#### Root `docker-compose.yml`:
```yaml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: notebook
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d notebook"]
      interval: 2s
      timeout: 2s
      retries: 20
    ports:
      - "5432:5432"

  backend:
    build:
      context: ./backend
    command: ["sh", "-c", "npm install --no-audit --no-fund && npm run start"]
    environment:
      PORT: "3000"
      DATABASE_URL: "postgres://postgres:postgres@db:5432/notebook"
      JWT_SECRET: "e2e-test-jwt-secret"
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "3000:3000"

  frontend:
    build:
      context: ./frontend
    command: ["sh", "-c", "npm install --no-audit --no-fund && npm run build && npx serve -s dist -l 5173"]
    environment:
      VITE_API_URL: "http://backend:3000"
      VITE_WS_URL: "ws://backend:3000"
    depends_on:
      - backend
    ports:
      - "5173:5173"

  e2e:
    build: ./tests/e2e
    depends_on:
      - backend
      - frontend
    environment:
      API_URL: "http://backend:3000"
      FRONTEND_URL: "http://frontend:5173"
      WS_URL: "ws://backend:3000"
```

#### `tests/e2e/run_tests.sh` Black-Box Assertions:
The black-box E2E test runner container executes standard `curl` and Node.js WebSocket client assertions:
1. `GET ${API_URL}/healthz` returns `200 {"status":"ok"}`.
2. User registration (`POST /api/v1/auth/register`) returns `201` and a valid JWT token.
3. Authenticated `POST /api/v1/notes` creates a note.
4. WebSocket client connects to `${WS_URL}/ws?token=<jwt>`.
5. Updating the note (`PUT /api/v1/notes/:id`) triggers a `NOTE_UPDATED` WebSocket frame sent to the connected socket.
6. `GET ${FRONTEND_URL}` returns `200` with HTML containing the React root (`<div id="root">`).
7. Exits `0` on total success or `1` on any failure.

---

## 10. Documentation (`README.md`)

`README.md` must document: monorepo directory layout, installation (`make install`), running locally (`make start`), REST & WebSocket contracts, database schema, and test execution (`make test`, `make lint`, `make e2e`).
