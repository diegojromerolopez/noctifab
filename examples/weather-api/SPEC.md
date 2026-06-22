# Weather Proxy API Specification

## 1. Overview
The goal of this project is to implement a Weather HTTP API proxy service. The service fetches weather data from a mock external weather provider, caches the results in memory to improve response times and bypass rate limits, and handles external failures by returning cached stale data or elegant error messages.

---

## 2. Requirements

### 2.1. Endpoints
The service must expose the following HTTP endpoints:

1.  **GET `/weather`**
    *   **Query Parameters:** `city` (string, required).
    *   **Behavior:**
        *   If the requested city's weather is in the local cache and not expired (expiry is 10 minutes), return the cached response.
        *   If the city's weather is not in the cache or is expired, fetch the latest data from the upstream weather provider.
        *   Update the cache with the fresh response.
        *   If the upstream request fails (e.g. network timeout or 5xx error) and a cached entry exists (even if expired), return the stale cached data with an HTTP status `200` and an extra header or field indicating stale data (`"stale": true`).
        *   If the upstream request fails and no cache entry exists, return a `502 Bad Gateway` error.
    *   **Response Body (JSON - Success, 200 OK):**
        ```json
        {
          "city": "London",
          "temperature": 18.5,
          "condition": "Cloudy",
          "cached": true,
          "stale": false,
          "fetched_at": "2026-06-21T23:20:00Z"
        }
        ```
    *   **Response (Error, 400 Bad Request):** If the `city` parameter is missing.

2.  **GET `/cache/status`**
    *   **Description:** Returns internal caching metrics.
    *   **Response Body (JSON - Success, 200 OK):**
        ```json
        {
          "hits": 14,
          "misses": 3,
          "cached_items_count": 2
        }
        ```

### 2.2. Upstream Weather API
*   The proxy should query a mock external endpoint or a public endpoint (such as Open-Meteo).
*   For testing purposes, the base URL of this upstream service must be configurable via an environment variable `UPSTREAM_API_URL` or configuration parameter.

### 2.3. Cache Design
*   The cache must be implemented in-memory (e.g. using a map/dictionary with lock synchronization or equivalent thread-safe data structures).
*   Each item in the cache must have an expiration TTL of 10 minutes.

---

## 3. Technical Constraints
*   **Language:** Go, Python, or Node.js.
*   **Dependencies:** Standard libraries are preferred.
*   **Cache Store:** Strict in-memory cache to prevent external server dependencies.

---

## 4. Verification Criteria & Testing

### 4.1. Expected Tests
*   **Unit & Integration Tests:**
    *   Verify cache hit and cache miss counts increment correctly.
    *   Verify the cache expiration (TTL) removes or invalidates items after the elapsed time (mocking time is highly recommended).
    *   Verify proxy routing and URL construction to the upstream API.
    *   **Resiliency Tests:** Mock the upstream API using a test server. Verify that if the upstream API returns `500` or times out, the proxy successfully serves stale cached data when available, and returns `502` when cache is empty.
