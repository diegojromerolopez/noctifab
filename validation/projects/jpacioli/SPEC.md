# `jpacioli` Technical Specification & Architecture

`jpacioli` is a high-precision, enterprise-grade **Double-Entry Financial Accounting Ledger and Transaction Engine** built with **Java 21, Spring Boot 3.3+, Domain-Driven Design (DDD), Event-Driven Architecture (EDA)**, and a **Role-Based Access Control (RBAC) Permission System** backed by **PostgreSQL**.

Named after **Luca Pacioli**—the 15th-century mathematician who codified double-entry bookkeeping in 1494—this service serves as the core system of record for account balances, multi-leg financial journals, idempotent money transfers, transactional outbox event streams, and cryptographic audit trails.

---

## 1. Core Financial Invariants & Architectural Rules

> [!IMPORTANT]
> **RULE 1: THE FUNDAMENTAL DOUBLE-ENTRY EQUATION**
> Every financial transaction posted to the ledger consists of two or more **Journal Entries (legs)**.
> The algebraic sum of all debits must strictly equal the sum of all credits for every transaction:
> $$\sum \text{Debits} = \sum \text{Credits} \iff \sum (\text{Debit Amounts}) - \sum (\text{Credit Amounts}) = 0$$
> Any attempt to post an unbalanced transaction MUST be rejected with HTTP `422 Unprocessable Entity` (`UNBALANCED_TRANSACTION`).

> [!IMPORTANT]
> **RULE 2: ZERO FLOATING-POINT ARITHMETIC (EXACT DECIMAL PRECISION)**
> Floating-point types (`float`, `double`) are **strictly forbidden** for monetary amounts.
> All financial amounts must use `java.math.BigDecimal` (or an immutable `Money` Value Object) with explicit scale (typically 2 or 4 decimal places depending on currency) and ISO-4217 currency codes (e.g., `USD`, `EUR`, `GBP`).
> Rounding must strictly employ **Banker's Rounding** (`RoundingMode.HALF_EVEN`).

> [!IMPORTANT]
> **RULE 3: IMMUTABLE AUDIT TRAIL (APPEND-ONLY LEDGER)**
> Once a `Transaction` and its `JournalEntry` legs are posted and committed to the database, they are **strictly immutable**.
> Direct `UPDATE` or `DELETE` operations on journal records are prohibited.
> Adjustments, corrections, and reversals must be executed by posting a new compensating transaction referencing the original transaction ID.

> [!IMPORTANT]
> **RULE 4: STRICT IDEMPOTENCY & TRANSACTION ISOLATION**
> Financial operations (`POST /api/v1/transactions`, `POST /api/v1/transfers`) MUST require an `Idempotency-Key` header or request attribute.
> Duplicate requests with the same idempotency key must safely return the original transaction result without re-executing ledger state modifications.
> Concurrent balance updates on accounts must use optimistic concurrency (`@Version`) or pessimistic row-level locking (`SELECT ... FOR UPDATE`) to prevent race conditions and overdrafts.

> [!IMPORTANT]
> **RULE 5: TRANSACTIONAL OUTBOX PATTERN (EVENT-DRIVEN ARCHITECTURE)**
> State mutations (e.g., Account creation, Transaction posting, Status changes) MUST emit domain events (`AccountCreatedEvent`, `TransactionPostedEvent`, `TransferCompletedEvent`).
> Domain events MUST be persisted atomically in an `outbox_events` table within the **exact same database transaction** as the business entity changes, guaranteeing at-least-once event delivery.

> [!IMPORTANT]
> **RULE 6: STATELESS JWT AUTHENTICATION & FINE-GRAINED PERMISSION SYSTEM (RBAC)**
> All API endpoints (except `/actuator/health` and `/api/v1/auth/token`) MUST enforce stateless **JWT Bearer Token** authentication.
> Authorization follows a strict Role-Based Access Control (RBAC) and Permission matrix. Missing/expired tokens return HTTP `401 Unauthorized`; insufficient permissions return HTTP `403 Forbidden`.

---

## 2. Authentication, Roles & Permissions System

### 2.1 Security Roles
1. **`ROLE_SUPERADMIN`**: Master root administrator; possesses unrestricted global access to create new entries and transactions, open/freeze accounts, manage users, and inspect the entire system state (all accounts, statements, audit logs, and outbox streams) with superadmin credentials.
2. **`ROLE_ADMIN`**: Administrative authority; manages users, accounts, and system configuration.
3. **`ROLE_ACCOUNTANT`**: Financial operations; creates accounts, posts multi-leg transactions, executes transfers, and queries statements.
4. **`ROLE_AUDITOR`**: Read-only compliance officer; can inspect accounts, balances, journals, statements, and outbox event streams, but CANNOT mutate ledger state.
5. **`ROLE_OPERATOR`**: Machine-to-machine integration role for automated transfers and balance checks.

### 2.2 Permissions Matrix

| Permission Code | Description | Roles Possessing Permission |
| :--- | :--- | :--- |
| **`accounts:read`** | View account details, balances, and statements | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `AUDITOR`, `OPERATOR` |
| **`accounts:write`** | Create accounts, freeze accounts, update metadata | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT` |
| **`transactions:read`** | View transaction details, journals, and history | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `AUDITOR`, `OPERATOR` |
| **`transactions:write`** | Post multi-leg journal entries and execute transfers | `SUPERADMIN`, `ADMIN`, `ACCOUNTANT`, `OPERATOR` |
| **`outbox:read`** | Query and inspect domain event streams from outbox | `SUPERADMIN`, `ADMIN`, `AUDITOR` |
| **`users:manage`** | Create users and assign security roles | `SUPERADMIN`, `ADMIN` |

### 2.3 Pre-Seeded Default Test Credentials
For black-box test automation and validation harnesses, the system initializes with:

| Username | Password | Role | Permissions |
| :--- | :--- | :--- | :--- |
| `superadmin` | `SuperAdmin123!` | `ROLE_SUPERADMIN` | `*` (Unrestricted: create new entries, manage accounts/users, view all state) |
| `admin` | `Admin123!` | `ROLE_ADMIN` | `*` (All standard admin permissions) |
| `accountant` | `Accountant123!` | `ROLE_ACCOUNTANT` | `accounts:*`, `transactions:*` |
| `auditor` | `Auditor123!` | `ROLE_AUDITOR` | `accounts:read`, `transactions:read`, `outbox:read` |
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

## 4. Package & Directory Structure (DDD Layered Architecture)

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
│   │   │   ├── domain/                         # Pure Domain Layer (Entities, Value Objects, Domain Events)
│   │   │   │   ├── model/
│   │   │   │   │   ├── Account.java            # Account aggregate root
│   │   │   │   │   ├── AccountId.java          # Strongly typed ID (UUID)
│   │   │   │   │   ├── AccountType.java        # ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
│   │   │   │   │   ├── AccountStatus.java      # ACTIVE, FROZEN, CLOSED
│   │   │   │   │   ├── Transaction.java        # Transaction aggregate root
│   │   │   │   │   ├── TransactionId.java      # Strongly typed ID (UUID)
│   │   │   │   │   ├── JournalEntry.java       # Individual ledger entry leg
│   │   │   │   │   ├── EntryType.java          # DEBIT, CREDIT
│   │   │   │   │   ├── Money.java              # Immutable Value Object (BigDecimal + Currency)
│   │   │   │   │   ├── Currency.java           # ISO-4217 Currency Value Object
│   │   │   │   │   ├── User.java               # Security User aggregate root
│   │   │   │   │   ├── UserId.java             # Strongly typed ID (UUID)
│   │   │   │   │   ├── Role.java               # ADMIN, ACCOUNTANT, AUDITOR, OPERATOR
│   │   │   │   │   └── Permission.java         # accounts:read, transactions:write, etc.
│   │   │   │   ├── event/
│   │   │   │   │   ├── DomainEvent.java        # Base event contract
│   │   │   │   │   ├── AccountCreatedEvent.java
│   │   │   │   │   ├── TransactionPostedEvent.java
│   │   │   │   │   └── AccountStatusChangedEvent.java
│   │   │   │   ├── repository/
│   │   │   │   │   ├── AccountRepository.java  # Domain repository interface
│   │   │   │   │   ├── TransactionRepository.java
│   │   │   │   │   ├── UserRepository.java
│   │   │   │   │   └── OutboxEventRepository.java
│   │   │   │   └── exception/
│   │   │   │       ├── DomainException.java
│   │   │   │       ├── UnbalancedTransactionException.java
│   │   │   │       ├── InsufficientFundsException.java
│   │   │   │       ├── InactiveAccountException.java
│   │   │   │       ├── CurrencyMismatchException.java
│   │   │   │       └── DuplicateIdempotencyKeyException.java
│   │   │   ├── application/                     # Application Services & Use Cases
│   │   │   │   ├── service/
│   │   │   │   │   ├── AccountApplicationService.java
│   │   │   │   │   ├── LedgerApplicationService.java
│   │   │   │   │   ├── AuthApplicationService.java
│   │   │   │   │   └── OutboxPublisherService.java
│   │   │   │   └── dto/
│   │   │   │       ├── AuthRequest.java
│   │   │   │       ├── AuthResponse.java
│   │   │   │       ├── CreateAccountCommand.java
│   │   │   │       ├── PostTransactionCommand.java
│   │   │   │       ├── TransferMoneyCommand.java
│   │   │   │       ├── AccountResponse.java
│   │   │   │       ├── TransactionResponse.java
│   │   │   │       └── OutboxEventResponse.java
│   │   │   └── infrastructure/                  # Adapters, Persistence, Security, REST
│   │   │       ├── persistence/
│   │   │       │   ├── entity/                 # JPA / Hibernate mapping entities
│   │   │       │   │   ├── AccountJpaEntity.java
│   │   │       │   │   ├── TransactionJpaEntity.java
│   │   │       │   │   ├── JournalEntryJpaEntity.java
│   │   │       │   │   ├── UserJpaEntity.java
│   │   │       │   │   └── OutboxEventJpaEntity.java
│   │   │       │   ├── repository/             # Spring Data Repositories & Adapters
│   │   │       │   │   ├── AccountRepositoryAdapter.java
│   │   │       │   │   ├── TransactionRepositoryAdapter.java
│   │   │       │   │   ├── UserRepositoryAdapter.java
│   │   │       │   │   └── OutboxEventRepositoryAdapter.java
│   │   │       │   └── mapper/                 # Entity-Domain Mappers
│   │   │       ├── security/
│   │   │       │   ├── SecurityConfig.java     # Spring Security Filter Chain
│   │   │       │   ├── JwtTokenProvider.java   # Token issuance, parsing, HMAC signing
│   │   │       │   ├── JwtAuthenticationFilter.java
│   │   │       │   └── UserDetailsServiceImpl.java
│   │   │       ├── web/
│   │   │       │   ├── AuthController.java
│   │   │       │   ├── AccountController.java
│   │   │       │   ├── TransactionController.java
│   │   │       │   ├── OutboxController.java
│   │   │       │   └── GlobalExceptionHandler.java  # RFC 7807 Problem Details
│   │   │       └── config/
│   │   │           └── DatabaseConfig.java
│   │   └── resources/
│   │       ├── application.yml
│   │       └── db/migration/
│   │           ├── V1__create_users_table.sql
│   │           ├── V2__create_accounts_table.sql
│   │           ├── V3__create_transactions_and_entries_tables.sql
│   │           ├── V4__create_outbox_events_table.sql
│   │           └── V5__seed_default_users.sql
│   └── test/
│       ├── java/com/jpacioli/
│       │   ├── domain/                         # Fast isolated unit tests (no DB/Spring)
│       │   │   ├── MoneyTest.java
│       │   │   ├── AccountTest.java
│       │   │   └── TransactionTest.java
│       │   ├── application/                    # Application service tests with mocked repositories
│       │   │   └── LedgerApplicationServiceTest.java
│       │   └── integration/                    # SpringBootTest + Flyway + PostgreSQL
│       │       ├── AuthApiIntegrationTest.java
│       │       ├── AccountApiIntegrationTest.java
│       │       ├── LedgerApiIntegrationTest.java
│       │       └── OutboxEventIntegrationTest.java
│       └── resources/
│           └── application-test.yml
└── tests/
    └── e2e/
        └── test_ledger_blackbox.sh             # Autonomous curl/HTTP black-box suite
```

---

## 5. Database Schema & Migrations (PostgreSQL)

### 5.1 `users` Table
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL,              -- ADMIN, ACCOUNTANT, AUDITOR, OPERATOR
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

### 5.2 `accounts` Table
```sql
CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    account_number VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(32) NOT NULL,              -- ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
    currency VARCHAR(3) NOT NULL,           -- ISO-4217 (USD, EUR, etc.)
    status VARCHAR(32) NOT NULL,            -- ACTIVE, FROZEN, CLOSED
    current_balance NUMERIC(19, 4) NOT NULL DEFAULT 0.0000,
    version BIGINT NOT NULL DEFAULT 0,      -- Optimistic locking
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

### 5.3 `transactions` & `journal_entries` Tables
```sql
CREATE TABLE transactions (
    id UUID PRIMARY KEY,
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    description TEXT NOT NULL,
    reference VARCHAR(128),
    currency VARCHAR(3) NOT NULL,
    total_amount NUMERIC(19, 4) NOT NULL,
    status VARCHAR(32) NOT NULL,            -- POSTED, REVERSED
    posted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE journal_entries (
    id UUID PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    entry_type VARCHAR(16) NOT NULL,        -- DEBIT, CREDIT
    amount NUMERIC(19, 4) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    entry_order INT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_journal_entries_account_id ON journal_entries(account_id);
CREATE INDEX idx_journal_entries_tx_id ON journal_entries(transaction_id);
```

### 5.4 `outbox_events` Table (Transactional Outbox)
```sql
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,    -- Account, Transaction
    aggregate_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(128) NOT NULL,       -- AccountCreated, TransactionPosted, etc.
    payload JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING', -- PENDING, PUBLISHED, FAILED
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_outbox_events_status ON outbox_events(status, created_at);
```

---

## 6. REST API Contract & Specifications

All API endpoints accept and return JSON with standard `Content-Type: application/json`.
Protected endpoints require header: `Authorization: Bearer <jwt-token>`.

### 6.1 `POST /api/v1/auth/token` — Authentication & JWT Token Issuance
* **Public Endpoint**
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
    "transactions:write"
  ]
}
```

### 6.2 `POST /api/v1/accounts` — Create Account
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
* **Response `201 Created`**:
```json
{
  "id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
  "account_number": "ACC-1001",
  "name": "Operating Cash Account",
  "type": "ASSET",
  "currency": "USD",
  "status": "ACTIVE",
  "current_balance": "0.00",
  "created_at": "2026-08-17T20:00:00Z"
}
```

### 6.3 `GET /api/v1/accounts/{id}` — Get Account by ID
* **Required Permission**: `accounts:read`
* **Response `200 OK`**:
```json
{
  "id": "7b8f9e0a-1c2d-3e4f-5a6b-7c8d9e0f1a2b",
  "account_number": "ACC-1001",
  "name": "Operating Cash Account",
  "type": "ASSET",
  "currency": "USD",
  "status": "ACTIVE",
  "current_balance": "1500.50",
  "version": 3,
  "created_at": "2026-08-17T20:00:00Z"
}
```

### 6.4 `POST /api/v1/transactions` — Multi-Leg Transaction Posting
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
* **Response `201 Created`**:
```json
{
  "id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
  "idempotency_key": "tx-idempotency-key-001",
  "description": "Invoice Payment Split with Service Fee",
  "reference": "INV-2026-889",
  "currency": "USD",
  "total_amount": "1000.00",
  "status": "POSTED",
  "posted_at": "2026-08-17T20:05:00Z",
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

### 6.5 `POST /api/v1/transfers` — Convenient 2-Leg Transfer Endpoint
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
* **Response `201 Created`**: Returns standard `TransactionResponse`.

### 6.6 `GET /api/v1/outbox/events` — Query Outbox Events
* **Required Permission**: `outbox:read`
* **Query Parameters**: `status=PENDING` or `status=PUBLISHED` (optional), `limit=50`
* **Response `200 OK`**:
```json
{
  "events": [
    {
      "id": "e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b",
      "aggregate_type": "Transaction",
      "aggregate_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
      "event_type": "TransactionPostedEvent",
      "status": "PENDING",
      "payload": {
        "transaction_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
        "total_amount": "1000.00",
        "currency": "USD",
        "posted_at": "2026-08-17T20:05:00Z"
      },
      "created_at": "2026-08-17T20:05:00Z"
    }
  ],
  "total": 1
}
```

### 6.7 Standard Error Envelope (RFC 7807 Problem Details)
All error responses return RFC 7807 compliant payloads:
```json
{
  "type": "https://jpacioli.dev/errors/forbidden",
  "title": "Access Denied",
  "status": 403,
  "detail": "Principal does not have required authority: transactions:write",
  "instance": "/api/v1/transactions",
  "timestamp": "2026-08-17T20:05:00Z"
}
```

---

## 7. Containerized Local & E2E Testing Harness (`docker-compose.yml` & `docker-compose.e2e.yml`)

The repository MUST include a containerized test harness featuring a dedicated `test-runner` container that verifies all endpoints in a live multi-container environment.

### 7.1 `docker-compose.e2e.yml`
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

### 7.2 `tests/e2e/Dockerfile`
```dockerfile
FROM alpine:3.21

RUN apk add --no-cache bash curl jq

WORKDIR /tests
COPY tests/e2e/test_ledger_blackbox.sh /tests/test_ledger_blackbox.sh
RUN chmod +x /tests/test_ledger_blackbox.sh

CMD ["/tests/test_ledger_blackbox.sh"]
```

---

## 8. Comprehensive Black-Box End-to-End Test Suite (`test_ledger_blackbox.sh`)

The `test-runner` container executes `test_ledger_blackbox.sh` against the live `jpacioli` service, testing **every public API endpoint** end-to-end:

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

3. Account Management (POST & GET /api/v1/accounts):
   - POST /api/v1/accounts (as auditor) -> 403 Forbidden
   - POST /api/v1/accounts (as accountant) -> 201 Created (Asset: ACC-CASH, USD)
   - POST /api/v1/accounts (as admin) -> 201 Created (Liability: ACC-LIAB, USD)
   - POST /api/v1/accounts (as superadmin) -> 201 Created (Revenue: ACC-REV, USD)
   - GET /api/v1/accounts/{id} (as auditor, accountant, superadmin) -> 200 OK (balance: 0.00)

4. Double-Entry Transactions (POST & GET /api/v1/transactions):
   - POST /api/v1/transactions (unbalanced: Debits != Credits) -> 422 Unprocessable Entity
   - POST /api/v1/transactions (as auditor) -> 403 Forbidden
   - POST /api/v1/transactions (as accountant with Idempotency-Key: tx-001) -> 201 Created
   - POST /api/v1/transactions (replay identical Idempotency-Key: tx-001) -> 200/201 without double balance mutation
   - POST /api/v1/transactions (as superadmin) -> 201 Created
   - GET /api/v1/transactions/{id} (as auditor, superadmin) -> 200 OK (verifying all journal legs)

5. Direct Money Transfers (POST /api/v1/transfers):
   - POST /api/v1/transfers (as operator) -> 201 Created (atomic 2-leg transfer)

6. Transactional Outbox Event Inspection (GET /api/v1/outbox/events):
   - GET /api/v1/outbox/events (as accountant) -> 403 Forbidden
   - GET /api/v1/outbox/events (as auditor & superadmin) -> 200 OK (verifying AccountCreatedEvent and TransactionPostedEvent payloads)
```

---

## 9. Makefile & Build Discipline

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

