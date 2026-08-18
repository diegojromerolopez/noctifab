# `jpacioli` Technical Specification & Architecture

`jpacioli` is an enterprise-grade **Double-Entry Financial Accounting Ledger and Transaction Engine** built with **Java 21, Spring Boot 3.3+, Full Event Sourcing (ES), Command Query Responsibility Segregation (CQRS)**, and a **Role-Based Access Control (RBAC) Permission System** backed by **PostgreSQL**.

Named after **Luca Pacioli**—the 15th-century Franciscan friar and mathematician who codified double-entry bookkeeping in 1494—this service represents the pure manifestation of historical double-entry accounting through modern Event Sourcing: **the Journal is the immutable Event Log**, and **the Ledger Balances are Materialized View Projections**.

---

## 1. Core Financial Invariants & Event Sourcing Rules

> [!IMPORTANT]
> **RULE 1: FULL EVENT SOURCING AS THE SINGLE SOURCE OF TRUTH**
> The state of all domain entities (e.g. `Account`, `Transaction`, `User`) is **never mutated in place**.
> The database `event_store` table is the **sole system of record**.
> Aggregates reconstruct their internal state exclusively by loading and replaying their immutable stream of historical domain events (`AccountCreatedEvent`, `AccountDebitedEvent`, `AccountCreditedEvent`, `AccountFrozenEvent`).
> Direct `UPDATE` or `DELETE` operations on historical domain events are strictly forbidden.

> [!IMPORTANT]
> **RULE 2: THE FUNDAMENTAL DOUBLE-ENTRY EQUATION**
> Every financial transaction posted to the ledger consists of two or more **Journal Entries (legs)**.
> The algebraic sum of all debits must strictly equal the sum of all credits for every transaction:
> $$\sum \text{Debits} = \sum \text{Credits} \iff \sum (\text{Debit Amounts}) - \sum (\text{Credit Amounts}) = 0$$
> Any command attempting to post an unbalanced transaction MUST be rejected with HTTP `422 Unprocessable Entity` (`UNBALANCED_TRANSACTION`).

> [!IMPORTANT]
> **RULE 3: ZERO FLOATING-POINT ARITHMETIC (EXACT DECIMAL PRECISION)**
> Floating-point types (`float`, `double`) are **strictly forbidden** for monetary amounts.
> All financial amounts must use `java.math.BigDecimal` (or an immutable `Money` Value Object) with explicit scale (typically 2 or 4 decimal places depending on currency) and ISO-4217 currency codes (e.g., `USD`, `EUR`, `GBP`).
> Rounding must strictly employ **Banker's Rounding** (`RoundingMode.HALF_EVEN`).

> [!IMPORTANT]
> **RULE 4: CQRS (COMMAND QUERY RESPONSIBILITY SEGREGATION)**
> The application strictly segregates the **Write Pipeline (Commands)** from the **Read Pipeline (Queries)**:
> - **Command Pipeline**: Validates business invariants against hydrated event-sourced Aggregates and appends new uncommitted events to the `event_store` with optimistic concurrency control (`expected_version`).
> - **Query Pipeline**: Asynchronous/synchronous Event Projectors consume committed events and maintain read-optimized materialized views (`account_balances_view`, `account_statements_view`, `trial_balance_view`). Queries NEVER read directly from or place locks on the `event_store`.

> [!IMPORTANT]
> **RULE 5: TIME-TRAVEL HISTORICAL BALANCE RECONSTRUCTION**
> Because all historical events are preserved with exact timestamps and monotonic stream versions, the system MUST support point-in-time historical balance queries:
> Replaying an aggregate's event stream up to timestamp $t$ reproduces the exact financial state of that account at that specific historical moment.

> [!IMPORTANT]
> **RULE 6: STRICT IDEMPOTENCY & STREAM OCC**
> Commands mutating financial state (`POST /api/v1/transactions`, `POST /api/v1/transfers`) MUST enforce an `Idempotency-Key` header.
> Concurrency collisions on the same aggregate event stream MUST be prevented via unique constraints on `(stream_id, event_version)` (Optimistic Concurrency Control).

> [!IMPORTANT]
> **RULE 7: STATELESS JWT AUTHENTICATION & RBAC PERMISSIONS**
> All API endpoints (except `/actuator/health` and `/api/v1/auth/token`) MUST enforce stateless **JWT Bearer Token** authentication.
> Authorization follows a strict Role-Based Access Control (RBAC) and Permission matrix. Missing/expired tokens return HTTP `401 Unauthorized`; insufficient permissions return HTTP `403 Forbidden`.

---

## 2. Authentication, Roles & Permissions System

### 2.1 Security Roles
1. **`ROLE_SUPERADMIN`**: Master root administrator; possesses unrestricted global access (`*`) to create transactions, open/freeze accounts, query historical balances and event streams, and manage users.
2. **`ROLE_ADMIN`**: Administrative authority; manages users, accounts, and system configuration.
3. **`ROLE_ACCOUNTANT`**: Financial operations; creates accounts, posts multi-leg transactions, executes transfers, and queries statements.
4. **`ROLE_AUDITOR`**: Read-only compliance officer; can inspect account balances, historical statements, trial balance reports, and event streams, but CANNOT mutate ledger state.
5. **`ROLE_OPERATOR`**: Machine-to-machine integration role for automated transfers and balance checks.

### 2.2 Permissions Matrix

| Permission Code | Description | Roles Possessing Permission |
| :--- | :--- | :--- |
| **`accounts:read`** | View account details, balances, statements, and time-travel history | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `AUDITOR`, `OPERATOR` |
| **`accounts:write`** | Create accounts, freeze accounts, update metadata | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT` |
| **`transactions:read`** | View transaction details, journals, and history | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `AUDITOR`, `OPERATOR` |
| **`transactions:write`** | Post multi-leg journal entries and execute transfers | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `OPERATOR` |
| **`events:read`** | Query raw event streams from the event store | `SUPERADMIN`, `ADMIN`, `AUDITOR` |
| **`reports:read`** | Generate consolidated Trial Balance and audit reports | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `AUDITOR` |
| **`users:manage`** | Create users and assign security roles | `SUPERADMIN`, `ADMIN` |

### 2.3 Pre-Seeded Default Test Credentials
For black-box test automation and validation harnesses, the system initializes with:

| Username | Password | Role | Permissions |
| :--- | :--- | :--- | :--- |
| `superadmin` | `SuperAdmin123!` | `ROLE_SUPERADMIN` | `*` (Unrestricted: create new entries, manage accounts/users, view all state) |
| `admin` | `Admin123!` | `ROLE_ADMIN` | `*` (All standard admin permissions) |
| `accountant` | `Accountant123!` | `ROLE_ACCOUNTANT` | `accounts:*`, `transactions:*`, `reports:read` |
| `auditor` | `Auditor123!` | `ROLE_AUDITOR` | `accounts:read`, `transactions:read`, `events:read`, `reports:read` |
| `operator` | `Operator123!` | `ROLE_OPERATOR` | `accounts:read`, `transactions:read`, `transactions:write` |

---

## 3. Domain Model & Account Classification

### 3.1 Account Types & Normal Balances
The ledger enforces the 5 standard accounting account types:

| Account Type | Normal Balance | Balance Formula | Examples |
| :--- | :---: | :--- | :--- |
| **`ASSET`** | Debit | $\text{Balance} = \sum \text{Debits} - \sum \text{Credits}$ | Bank accounts, Cash, Accounts Receivable |
| **`LIABILITY`** | Credit | $\text{Balance} = \sum \text{Credits} - \sum \text{Debits}$ | Loans, Accounts Payable, Customer Deposits |
| **`EQUITY`** | Credit | $\text{Balance} = \sum \text{Credits} - \sum \text{Debits}$ | Retained Earnings, Shareholder Capital |
| **`REVENUE`** | Credit | $\text{Balance} = \sum \text{Credits} - \sum \text{Debits}$ | Sales Revenue, Interest Income |
| **`EXPENSE`** | Debit | $\text{Balance} = \sum \text{Debits} - \sum \text{Credits}$ | Operating Expenses, Fees, Payroll |

### 3.2 Account Status
* **`ACTIVE`**: Normal operational state; accepts debits and credits.
* **`FROZEN`**: Temporarily suspended; rejects all debit transactions (credits may be allowed for settlement).
* **`CLOSED`**: Permanently closed; balance must be 0; rejects all postings.

---

## 4. Event Sourcing & CQRS Architecture

### 4.1 Aggregate Hydration Flow (Write Side)
```
1. Command Arrives (e.g. PostTransactionCommand)
2. Load all events for target Account(s) from `event_store` WHERE stream_id = accountId ORDER BY event_version ASC
3. Instantiate Account aggregate and replay events: account.apply(event)
4. Execute business method on Account (e.g. account.debit(amount)) -> verifies non-frozen, non-closed, currency match
5. Aggregate generates new uncommitted event (e.g. AccountDebitedEvent)
6. Transactional append: write new events to `event_store` with expected_version check
7. Projectors asynchronously or synchronously update materialized read views
```

### 4.2 Materialized Read Projections (Read Side)
* **`AccountBalanceProjector`**: Subscribes to `AccountCreatedEvent`, `AccountDebitedEvent`, `AccountCreditedEvent`, `AccountStatusChangedEvent` $\rightarrow$ updates `account_balances_view`.
* **`AccountStatementProjector`**: Subscribes to `AccountDebitedEvent`, `AccountCreditedEvent` $\rightarrow$ appends rows to `account_statements_view` maintaining running balance.
* **`TrialBalanceProjector`**: Maintains aggregated sums of Debits and Credits partitioned by `account_type`.

---

## 5. Package & Directory Structure

```
jpacioli/
├── build.gradle (or pom.xml)
├── Makefile
├── Dockerfile
├── docker-compose.yml
├── docker-compose.e2e.yml
├── src/
│   ├── main/
│   │   ├── java/com/jpacioli/
│   │   │   ├── JpacioliApplication.java
│   │   │   ├── domain/                         # Pure Domain Layer (Event-Sourced Aggregates & Events)
│   │   │   │   ├── core/                       # Event Sourcing Base Building Blocks
│   │   │   │   │   ├── AggregateRoot.java      # Base class tracking uncommitted events & version
│   │   │   │   │   ├── DomainEvent.java        # Base immutable event interface
│   │   │   │   │   └── EventStore.java         # Domain EventStore port interface
│   │   │   │   ├── model/
│   │   │   │   │   ├── Account.java            # Event-sourced Account aggregate
│   │   │   │   │   ├── AccountId.java          # Value Object (UUID)
│   │   │   │   │   ├── AccountType.java        # ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
│   │   │   │   │   ├── AccountStatus.java      # ACTIVE, FROZEN, CLOSED
│   │   │   │   │   ├── Transaction.java        # Event-sourced Transaction aggregate
│   │   │   │   │   ├── TransactionId.java      # Value Object (UUID)
│   │   │   │   │   ├── JournalEntry.java       # Individual ledger entry leg
│   │   │   │   │   ├── EntryType.java          # DEBIT, CREDIT
│   │   │   │   │   ├── Money.java              # Immutable Value Object (BigDecimal + Currency)
│   │   │   │   │   ├── Currency.java           # ISO-4217 Currency Value Object
│   │   │   │   │   ├── User.java               # Security User entity
│   │   │   │   │   ├── Role.java               # SUPERADMIN, ADMIN, ACCOUNTANT, AUDITOR, OPERATOR
│   │   │   │   │   └── Permission.java         # accounts:read, transactions:write, etc.
│   │   │   │   ├── event/                      # Immutable Domain Events
│   │   │   │   │   ├── AccountCreatedEvent.java
│   │   │   │   │   ├── AccountDebitedEvent.java
│   │   │   │   │   ├── AccountCreditedEvent.java
│   │   │   │   │   ├── AccountStatusChangedEvent.java
│   │   │   │   │   ├── TransactionPostedEvent.java
│   │   │   │   │   └── UserCreatedEvent.java
│   │   │   │   └── exception/
│   │   │   │       ├── DomainException.java
│   │   │   │       ├── ConcurrencyException.java
│   │   │   │       ├── UnbalancedTransactionException.java
│   │   │   │       ├── InsufficientFundsException.java
│   │   │   │       ├── InactiveAccountException.java
│   │   │   │       ├── CurrencyMismatchException.java
│   │   │   │       └── DuplicateIdempotencyKeyException.java
│   │   │   ├── application/                     # CQRS Command & Query Handlers
│   │   │   │   ├── command/                    # Command Side (Mutations)
│   │   │   │   │   ├── CreateAccountCommand.java
│   │   │   │   │   ├── PostTransactionCommand.java
│   │   │   │   │   ├── TransferMoneyCommand.java
│   │   │   │   │   ├── FreezeAccountCommand.java
│   │   │   │   │   └── handler/
│   │   │   │   │       ├── AccountCommandHandler.java
│   │   │   │   │       └── TransactionCommandHandler.java
│   │   │   │   ├── query/                      # Query Side (Materialized View Readers)
│   │   │   │   │   ├── GetAccountByIdQuery.java
│   │   │   │   │   ├── GetAccountStatementQuery.java
│   │   │   │   │   ├── GetHistoricalBalanceQuery.java
│   │   │   │   │   ├── GetTrialBalanceQuery.java
│   │   │   │   │   └── handler/
│   │   │   │   │       ├── AccountQueryHandler.java
│   │   │   │   │       └── LedgerQueryHandler.java
│   │   │   │   ├── projection/                 # Read-Model Event Projectors
│   │   │   │   │   ├── AccountBalanceProjector.java
│   │   │   │   │   ├── AccountStatementProjector.java
│   │   │   │   │   └── TrialBalanceProjector.java
│   │   │   │   └── dto/
│   │   │   │       ├── AuthRequest.java
│   │   │   │       ├── AuthResponse.java
│   │   │   │       ├── AccountResponse.java
│   │   │   │       ├── TransactionResponse.java
│   │   │   │       ├── AccountStatementResponse.java
│   │   │   │       ├── TrialBalanceResponse.java
│   │   │   │       └── EventStreamResponse.java
│   │   │   └── infrastructure/                  # Adapters, Persistence, Security, Web
│   │   │       ├── eventstore/                 # PostgreSQL Event Store Adapter
│   │   │       │   ├── PostgresEventStore.java
│   │   │       │   ├── EventStoreEntity.java
│   │   │       │   └── EventStoreRepository.java
│   │   │       ├── projection/                 # JPA Materialized View Repositories
│   │   │       │   ├── entity/
│   │   │       │   │   ├── AccountBalanceViewEntity.java
│   │   │       │   │   └── AccountStatementViewEntity.java
│   │   │       │   └── repository/
│   │   │       │       ├── AccountBalanceViewRepository.java
│   │   │       │       └── AccountStatementViewRepository.java
│   │   │       ├── security/
│   │   │       │   ├── SecurityConfig.java     # Spring Security Filter Chain
│   │   │       │   ├── JwtTokenProvider.java   # Token issuance, parsing, HMAC signing
│   │   │       │   ├── JwtAuthenticationFilter.java
│   │   │       │   └── UserDetailsServiceImpl.java
│   │   │       └── web/
│   │   │           ├── AuthController.java
│   │   │           ├── AccountCommandController.java
│   │   │           ├── AccountQueryController.java
│   │   │           ├── TransactionCommandController.java
│   │   │           ├── LedgerQueryController.java
│   │   │           └── GlobalExceptionHandler.java  # RFC 7807 Problem Details
│   │   └── resources/
│   │       ├── application.yml
│   │       └── db/migration/
│   │           ├── V1__create_event_store_table.sql
│   │           ├── V2__create_users_table.sql
│   │           ├── V3__create_account_balances_view_table.sql
│   │           ├── V4__create_account_statements_view_table.sql
│   │           ├── V5__create_snapshots_table.sql
│   │           └── V6__seed_default_users.sql
│   └── test/
│       ├── java/com/jpacioli/
│       │   ├── domain/                         # Isolated aggregate event hydration tests
│       │   │   ├── AccountEventSourcingTest.java
│       │   │   ├── TransactionEventSourcingTest.java
│       │   │   └── MoneyTest.java
│       │   ├── application/                    # Command & Query Handler tests
│       │   │   ├── AccountCommandHandlerTest.java
│       │   │   └── TransactionCommandHandlerTest.java
│       │   └── integration/                    # SpringBootTest + Flyway + PostgreSQL
│       │       ├── EventStoreIntegrationTest.java
│       │       ├── AccountCQRSIntegrationTest.java
│       │       ├── LedgerCQRSIntegrationTest.java
│       │       └── TimeTravelBalanceIntegrationTest.java
│       └── resources/
│           └── application-test.yml
└── tests/
    └── e2e/
        ├── Dockerfile                          # E2E test-runner container
        └── test_ledger_blackbox.sh             # Full API suite
```

---

## 6. Database Schema & Migrations (PostgreSQL)

### 6.1 `event_store` Table (Single Source of Truth)
```sql
CREATE TABLE event_store (
    event_id UUID PRIMARY KEY,
    stream_id UUID NOT NULL,
    aggregate_type VARCHAR(64) NOT NULL,    -- Account, Transaction, User
    event_type VARCHAR(128) NOT NULL,       -- AccountCreatedEvent, AccountDebitedEvent, etc.
    event_version BIGINT NOT NULL,          -- Monotonic stream sequence (1, 2, 3...)
    payload JSONB NOT NULL,                 -- Full event payload
    metadata JSONB,                         -- CorrelationId, CausationId, UserId
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stream_version UNIQUE (stream_id, event_version)
);

CREATE INDEX idx_event_store_stream ON event_store(stream_id, event_version);
CREATE INDEX idx_event_store_type_created ON event_store(aggregate_type, created_at);
```

### 6.2 `account_balances_view` Table (CQRS Read Projection)
```sql
CREATE TABLE account_balances_view (
    account_id UUID PRIMARY KEY,
    account_number VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(32) NOT NULL,              -- ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(32) NOT NULL,            -- ACTIVE, FROZEN, CLOSED
    current_balance NUMERIC(19, 4) NOT NULL DEFAULT 0.0000,
    version BIGINT NOT NULL DEFAULT 0,
    last_event_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

### 6.3 `account_statements_view` Table (CQRS Statement Projection)
```sql
CREATE TABLE account_statements_view (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES account_balances_view(account_id) ON DELETE CASCADE,
    transaction_id UUID NOT NULL,
    entry_type VARCHAR(16) NOT NULL,        -- DEBIT, CREDIT
    amount NUMERIC(19, 4) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    running_balance NUMERIC(19, 4) NOT NULL,
    description TEXT NOT NULL,
    posted_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_statements_account_posted ON account_statements_view(account_id, posted_at DESC);
```

### 6.4 `users` Table
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

---

## 7. REST API Contract & Specifications

All protected endpoints require header: `Authorization: Bearer <jwt-token>`.

### 7.1 `POST /api/v1/auth/token` — Authentication & JWT Issuance (Public)
* **Request**:
```json
{
  "username": "accountant",
  "password": "Accountant123!"
}
```
* **Response `200 OK`**:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsIn...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "role": "ROLE_ACCOUNTANT",
  "permissions": [
    "accounts:read",
    "accounts:write",
    "transactions:read",
    "transactions:write",
    "reports:read"
  ]
}
```

### 7.2 `POST /api/v1/accounts` — Create Account (Command)
* **Required Permission**: `accounts:write`
* **Request**:
```json
{
  "account_number": "ACC-1001",
  "name": "Operating Cash Account",
  "type": "ASSET",
  "currency": "USD"
}
```
* **Response `201 Created`**: Returns created `AccountResponse`.

### 7.3 `GET /api/v1/accounts/{id}` — Get Current Balance (Query)
* **Required Permission**: `accounts:read`
* **Response `200 OK`**: Reads from `account_balances_view`.
```json
{
  "id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
  "account_number": "ACC-1001",
  "name": "Operating Cash Account",
  "type": "ASSET",
  "currency": "USD",
  "status": "ACTIVE",
  "current_balance": "1500.00",
  "version": 4,
  "last_event_at": "2026-08-18T08:00:00Z"
}
```

### 7.4 `GET /api/v1/accounts/{id}/balance?at={timestamp}` — Point-in-Time Historical Balance (Query)
* **Required Permission**: `accounts:read`
* **Description**: Replays `Account` event stream up to timestamp `at` to compute historical balance.
* **Response `200 OK`**:
```json
{
  "account_id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
  "currency": "USD",
  "historical_balance": "500.00",
  "as_of": "2026-08-15T12:00:00Z",
  "events_replayed": 2
}
```

### 7.5 `GET /api/v1/accounts/{id}/statement` — Paginated Statement (Query)
* **Required Permission**: `accounts:read`
* **Query Parameters**: `page=0&size=20`
* **Response `200 OK`**:
```json
{
  "entries": [
    {
      "id": "c1d2e3f4-a5b6-7c8d-9e0f-1a2b3c4d5e6f",
      "transaction_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
      "entry_type": "DEBIT",
      "amount": "1000.00",
      "currency": "USD",
      "running_balance": "1000.00",
      "description": "Initial operating transfer",
      "posted_at": "2026-08-18T08:05:00Z"
    }
  ],
  "page": 0,
  "total": 1
}
```

### 7.6 `POST /api/v1/transactions` — Multi-Leg Transaction (Command)
* **Required Permission**: `transactions:write`
* **Headers**: `Idempotency-Key: tx-idempotency-key-001`
* **Request**:
```json
{
  "description": "Invoice Payment Split with Service Fee",
  "reference": "INV-2026-889",
  "currency": "USD",
  "entries": [
    {
      "account_id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
      "entry_type": "DEBIT",
      "amount": "1000.00"
    },
    {
      "account_id": "8c9a0b1c-2d3e-4f5a-6b7c-8d9e0f1a2b3c",
      "entry_type": "CREDIT",
      "amount": "950.00"
    },
    {
      "account_id": "9d0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d",
      "entry_type": "CREDIT",
      "amount": "50.00"
    }
  ]
}
```
* **Response `201 Created`**: Returns `TransactionResponse`.

### 7.7 `POST /api/v1/transfers` — 2-Leg Transfer Endpoint (Command)
* **Required Permission**: `transactions:write`
* **Headers**: `Idempotency-Key: transfer-uuid-99`
* **Request**:
```json
{
  "source_account_id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
  "destination_account_id": "8c9a0b1c-2d3e-4f5a-6b7c-8d9e0f1a2b3c",
  "amount": "250.00",
  "currency": "USD",
  "description": "Internal account rebalance transfer"
}
```
* **Response `201 Created`**: Returns `TransactionResponse`.

### 7.8 `GET /api/v1/reports/trial-balance` — Trial Balance Report (Query)
* **Required Permission**: `reports:read`
* **Response `200 OK`**:
```json
{
  "as_of": "2026-08-18T08:10:00Z",
  "total_debits": "10000.00",
  "total_credits": "10000.00",
  "is_balanced": true,
  "currency": "USD",
  "accounts": [
    {
      "account_number": "ACC-1001",
      "name": "Operating Cash Account",
      "type": "ASSET",
      "debit_balance": "10000.00",
      "credit_balance": "0.00"
    }
  ]
}
```

### 7.9 `GET /api/v1/events` — Global Event Stream Inspection (Query)
* **Required Permission**: `events:read`
* **Query Parameters**: `stream_id={uuid}` (optional), `limit=50`
* **Response `200 OK`**:
```json
{
  "events": [
    {
      "event_id": "e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b",
      "stream_id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
      "aggregate_type": "Account",
      "event_type": "AccountCreatedEvent",
      "event_version": 1,
      "payload": {
        "account_number": "ACC-1001",
        "type": "ASSET",
        "currency": "USD"
      },
      "created_at": "2026-08-18T08:00:00Z"
    }
  ],
  "total": 1
}
```

---

## 8. Containerized Local & E2E Testing Harness (`docker-compose.e2e.yml`)

### 8.1 `docker-compose.e2e.yml`
```yaml
version: '3.8'

services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: jpacioli
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d jpacioli"]
      interval: 3s
      timeout: 3s
      retries: 10

  jpacioli:
    build:
      context: .
      dockerfile: Dockerfile
    environment:
      SPRING_DATASOURCE_URL: jdbc:postgresql://db:5432/jpacioli
      SPRING_DATASOURCE_USERNAME: postgres
      SPRING_DATASOURCE_PASSWORD: postgres
      SUPERADMIN_PASSWORD: SuperAdmin123!
    ports:
      - "8080:8080"
    depends_on:
      db:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:8080/actuator/health || exit 1"]
      interval: 5s
      timeout: 3s
      retries: 15
      start_period: 20s

  test-runner:
    build:
      context: .
      dockerfile: tests/e2e/Dockerfile
    environment:
      JPACIOLI_URL: http://jpacioli:8080
      SUPERADMIN_PASSWORD: SuperAdmin123!
    depends_on:
      jpacioli:
        condition: service_healthy
```

### 8.2 `tests/e2e/Dockerfile`
```dockerfile
FROM alpine:3.21

RUN apk add --no-cache bash curl jq

WORKDIR /tests
COPY tests/e2e/test_ledger_blackbox.sh /tests/test_ledger_blackbox.sh
RUN chmod +x /tests/test_ledger_blackbox.sh

CMD ["/tests/test_ledger_blackbox.sh"]
```

---

## 9. Comprehensive Black-Box End-to-End Test Suite (`test_ledger_blackbox.sh`)

The `test-runner` container executes `test_ledger_blackbox.sh` against the live `jpacioli` service, testing **every public API endpoint and Event Sourcing invariant** end-to-end:

```
[TEST MATRIX COVERAGE]
1. Health & Unauthenticated Access:
   - GET /actuator/health -> 200 UP (public)
   - GET /api/v1/accounts -> 401 Unauthorized (missing token)

2. Auth & Token Issuance (POST /api/v1/auth/token):
   - Bad Credentials -> 401 Unauthorized
   - Valid superadmin / SuperAdmin123! -> 200 OK (ROLE_SUPERADMIN, all permissions)
   - Valid accountant / Accountant123! -> 200 OK (ROLE_ACCOUNTANT)
   - Valid auditor / Auditor123! -> 200 OK (ROLE_AUDITOR)
   - Valid operator / Operator123! -> 200 OK (ROLE_OPERATOR)

3. Account Management Commands & Projections (POST & GET /api/v1/accounts):
   - POST /api/v1/accounts (as auditor) -> 403 Forbidden
   - POST /api/v1/accounts (as accountant) -> 201 Created (Asset: ACC-CASH, USD)
   - POST /api/v1/accounts (as admin) -> 201 Created (Liability: ACC-LIAB, USD)
   - POST /api/v1/accounts (as superadmin) -> 201 Created (Revenue: ACC-REV, USD)
   - GET /api/v1/accounts/{id} (as auditor, accountant, superadmin) -> 200 OK (balance: 0.00 from projected view)

4. Double-Entry Transactions (POST & GET /api/v1/transactions):
   - POST /api/v1/transactions (unbalanced: Debits != Credits) -> 422 Unprocessable Entity
   - POST /api/v1/transactions (as auditor) -> 403 Forbidden
   - POST /api/v1/transactions (as accountant with Idempotency-Key: tx-001) -> 201 Created
   - POST /api/v1/transactions (replay identical Idempotency-Key: tx-001) -> 200/201 without duplicate event appending
   - POST /api/v1/transactions (as superadmin) -> 201 Created
   - GET /api/v1/transactions/{id} (as auditor, superadmin) -> 200 OK

5. Direct Money Transfers (POST /api/v1/transfers):
   - POST /api/v1/transfers (as operator) -> 201 Created (atomic multi-event append)

6. Point-in-Time Historical Balance & Statements (GET /api/v1/accounts/{id}/balance & /statement):
   - GET /api/v1/accounts/{id}/balance?at=<past_timestamp> -> returns historical balance replayed from event store
   - GET /api/v1/accounts/{id}/statement -> returns paginated statement entries with running balance

7. Trial Balance Report (GET /api/v1/reports/trial-balance):
   - GET /api/v1/reports/trial-balance (as accountant & auditor) -> 200 OK (Total Debits == Total Credits, is_balanced: true)

8. Event Stream Inspection (GET /api/v1/events):
   - GET /api/v1/events (as accountant) -> 403 Forbidden
   - GET /api/v1/events (as auditor & superadmin) -> 200 OK (verifying AccountCreatedEvent and AccountDebitedEvent in event store)
```

---

## 10. Makefile & Build Discipline

The root `Makefile` must implement the standard validation lifecycle:

```makefile
.PHONY: build test lint format e2e clean

build:
	gradle build -x test

test:
	gradle test

lint:
	gradle check

format:
	gradle spotlessApply 2>/dev/null || true

e2e:
	docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner

clean:
	gradle clean
```
