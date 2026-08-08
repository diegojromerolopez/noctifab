# `auth-vault` Technical Specification & Architecture

`auth-vault` is a production-grade, zero-trust OAuth2 / OpenID Connect (OIDC) Authorization Server and PKI Key Management Vault written in Go 1.22+. It provides RFC 7636 PKCE authorization code flows, OAuth2 client credentials flows, RS256/ES256 JWKS public key rotation, RFC 7662 token introspection, RFC 7009 token revocation, OIDC discovery, and an internal PKI engine for issuing short-lived client certificates and scoped API tokens.

---

## 1. Core Technical Invariants & Security Boundaries

> [!IMPORTANT]
> **GOAL 1: STRICT OAUTH2 & OIDC CONFORMANCE**
> - **RFC 7636 PKCE Mandate**: Standard authorization code flows MUST enforce Proof Key for Code Exchange with `code_challenge_method=S256`. Requests with `plain` or missing PKCE challenges MUST be rejected with HTTP 400 (`invalid_request`).
> - **OIDC Discovery & JWKS**: Must expose standard OIDC metadata at `/.well-known/openid-configuration` and public JSON Web Key Sets at `/.well-known/jwks.json`.
> - **Cryptographic Key Rotation**: Active signing keys MUST rotate periodically or via administrative triggers, retaining previous public keys in `jwks.json` to allow ongoing verification of non-expired tokens.

> [!IMPORTANT]
> **GOAL 2: ZERO-TRUST TOKEN INTROSPECTION & REVOCATION**
> - **RFC 7662 Introspection**: `/oauth/introspect` MUST return active state, client_id, scope, exp, iat, sub, and iss fields for valid tokens, and `{"active": false}` for expired/revoked tokens.
> - **RFC 7009 Revocation**: Revoking a token MUST instantly invalidate both access and refresh tokens across all introspection queries and API authorization middleware checks.

> [!CAUTION]
> **GOAL 3: ZERO DEPENDENCY INJECTION VIOLATIONS & 500-LINE LIMIT**
> - No single `.go` source file may exceed **500 lines of code**.
> - Dependencies (token storage, key manager, clock, password hasher) MUST be supplied via constructors. Global singletons are strictly forbidden.

---

## 2. Directory & Package Structure

```
auth-vault/
├── main.go                       # Entry point, CLI flag parsing, HTTP server start
├── Makefile                      # build, test, lint, format, e2e targets
├── Dockerfile                    # Multi-stage Docker build for auth-vault
├── docker-compose.yml            # Local E2E testing harness (auth-vault + test runner)
├── pkg/
│   ├── domain/
│   │   ├── token.go              # Token, Scope, Claim, and Client domain entities
│   │   ├── key.go                # KeyPair, JWK, and JWKS domain entities
│   │   └── errors.go             # Standard OAuth2 error types
│   ├── infrastructure/
│   │   ├── memory_store.go       # Concurrent in-memory store for client credentials & tokens
│   │   └── pki_vault.go          # RS256/ES256 key generation, storage, and rotation
│   └── service/
│       ├── oauth_service.go      # PKCE verification, token issuance, refresh, revocation
│       └── oidc_service.go       # UserInfo, OIDC Discovery spec generator
├── api/
│   ├── router.go                 # HTTP router setup (Chi / stdlib net/http)
│   ├── handlers_oauth.go         # /oauth/authorize, /oauth/token, /oauth/introspect, /oauth/revoke
│   └── handlers_oidc.go          # /.well-known/openid-configuration, /.well-known/jwks.json, /userinfo
└── tests/
    ├── unit/                     # Unit tests for PKCE, JWKS rotation, token parsing
    ├── integration/              # HTTP API tests against test router
    └── e2e/                      # Black-box shell/curl e2e validation script
```

---

## 3. API Contract & Endpoints

### 3.1 `GET /.well-known/openid-configuration`
- **Response `200 OK`**:
```json
{
  "issuer": "http://localhost:8080",
  "authorization_endpoint": "http://localhost:8080/oauth/authorize",
  "token_endpoint": "http://localhost:8080/oauth/token",
  "userinfo_endpoint": "http://localhost:8080/userinfo",
  "jwks_uri": "http://localhost:8080/.well-known/jwks.json",
  "introspection_endpoint": "http://localhost:8080/oauth/introspect",
  "revocation_endpoint": "http://localhost:8080/oauth/revoke",
  "response_types_supported": ["code"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"],
  "code_challenge_methods_supported": ["S256"]
}
```

### 3.2 `GET /.well-known/jwks.json`
- **Response `200 OK`**:
```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "alg": "RS256",
      "kid": "key-2026-v1",
      "n": "<base64url-modulus>",
      "e": "AQAB"
    }
  ]
}
```

### 3.3 `POST /oauth/token`
- **Supported Grant Types**:
  1. `authorization_code`: Requires `code`, `redirect_uri`, `client_id`, `code_verifier`.
  2. `client_credentials`: Requires `client_id`, `client_secret` (or Basic Auth header).
  3. `refresh_token`: Requires `refresh_token`, `client_id`.
- **Response `200 OK`**:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "rt_8f9a2b...",
  "scope": "openid profile email"
}
```
- **Error Response `400 Bad Request`**:
```json
{
  "error": "invalid_grant",
  "error_description": "Invalid PKCE code_verifier or expired authorization code"
}
```

### 3.4 `POST /oauth/introspect`
- **Request Body**: `token=<access_or_refresh_token>`
- **Response `200 OK` (Active Token)**:
```json
{
  "active": true,
  "scope": "openid profile email",
  "client_id": "test-client",
  "sub": "user-123",
  "exp": 1775635200,
  "iat": 1775631600,
  "token_type": "Bearer"
}
```
- **Response `200 OK` (Inactive/Revoked Token)**:
```json
{
  "active": false
}
```

---

## 4. End-to-End Black-Box Verification (`docker-compose.e2e.yml`)

The project MUST include a `docker-compose.e2e.yml` configuration that spins up `auth-vault` and an `e2e-runner` container to test the authentication flow fully:

1. **Discovery & JWKS Verification**: Fetches `/.well-known/openid-configuration` and `/.well-known/jwks.json`.
2. **Client Credentials Flow**: Sends `client_id` and `client_secret` to `/oauth/token` and verifies JWT signature against the published JWKS keys.
3. **RFC 7636 PKCE Authorization Code Flow**:
   - Generates high-entropy `code_verifier` and SHA-256 `code_challenge`.
   - Requests authorization code at `/oauth/authorize`.
   - Exchanges code + verifier at `/oauth/token` to receive `access_token` and `refresh_token`.
   - Rejects token exchange if invalid/mismatched `code_verifier` is submitted.
4. **Token Introspection & Revocation**:
   - Calls `/oauth/introspect` verifying `active: true`.
   - Calls `/oauth/revoke` revoking the token.
   - Calls `/oauth/introspect` verifying `active: false`.

---

## 5. Local Testing & Verification Engine

### 5.1 Makefile Targets (REQUIRED)
- `make build` → Compiles binary into `bin/auth-vault`.
- `make run` → Runs `bin/auth-vault --port 8080`.
- `make test` → Runs unit & integration tests (`go test -v ./...`).
- `make lint` → Runs `go vet ./...` (or `golangci-lint run`).
- `make e2e` → Executes local Docker Compose E2E black-box test suite (`docker compose -f docker-compose.e2e.yml up --build --exit-code-from e2e-runner`).

### 5.2 Definition of Done (DoD) Criteria
1. **100% Pass Rate** on all unit and integration tests (`go test -v ./...`).
2. **Full E2E Auth Pass**: `docker compose -f docker-compose.e2e.yml up --build --exit-code-from e2e-runner` passes with exit code 0.
3. **PKCE S256 Enforcement**: Mismatched SHA256 verifiers return HTTP 400 `invalid_grant`.
4. **JWKS Rotation**: Key rotation adds a new active key while preserving old keys in JWKS.
5. **Zero Linter Findings** on `go vet ./...`.
