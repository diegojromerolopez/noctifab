# URL Shortener API Specification

## 1. Overview
The goal of this project is to implement a simple URL Shortener HTTP API service. The service takes a long URL, returns a short unique code, and redirects clients who request the short code back to the original URL.

---

## 2. Requirements

### 2.1. Endpoints
The service must expose the following HTTP endpoints:

1.  **POST `/shorten`**
    *   **Description:** Shortens a given URL.
    *   **Request Body (JSON):**
        ```json
        {
          "url": "https://example.com/some/long/path/here"
        }
        ```
    *   **Response Body (JSON - Success, 201 Created):**
        ```json
        {
          "short_code": "a1b2c3",
          "short_url": "http://localhost:8080/a1b2c3"
        }
        ```
    *   **Response (Error, 400 Bad Request):** If the input URL is empty or invalid.

2.  **GET `/{short_code}`**
    *   **Description:** Redirects the user to the original long URL.
    *   **Response (Success, 302 Found):** Sets the `Location` header to the original URL.
    *   **Response (Error, 404 Not Found):** If the code does not exist.

3.  **GET `/stats/{short_code}`**
    *   **Description:** Returns usage statistics for a shortened URL.
    *   **Response Body (JSON - Success, 200 OK):**
        ```json
        {
          "short_code": "a1b2c3",
          "url": "https://example.com/some/long/path/here",
          "clicks": 42,
          "created_at": "2026-06-21T23:00:00Z"
        }
        ```
    *   **Response (Error, 404 Not Found):** If the code does not exist.

### 2.2. Short Code Generation
*   Codes should be exactly 6 characters long.
*   Codes must contain alphanumeric characters (a-z, A-Z, 0-9).
*   Codes must be generated randomly or deterministically (e.g., using a hash of the URL with a salt) such that collision rates are minimal.

### 2.3. Persistence
*   Data must be persisted. Use SQLite or a local file-based database.
*   The SQLite database file name should be configurable or default to `urls.db`.

---

## 3. Technical Constraints
*   **Language:** Go, Python, or Node.js.
*   **Dependencies:** Standard libraries are preferred. If frameworks are used, keep them minimal (e.g., Gin/Mux for Go, Flask/FastAPI for Python, Express for Node.js).
*   **Database:** SQLite.

---

## 4. Verification Criteria & Testing

### 4.1. Expected Tests
*   **Unit Tests:** Verify that short code generation works and does not generate invalid characters.
*   **API Tests:**
    *   Verify POST `/shorten` returns `201` and a valid short code format.
    *   Verify GET `/{short_code}` returns `302` with the correct `Location` header.
    *   Verify GET `/stats/{short_code}` tracks clicks and increments them on redirection.
    *   Verify negative cases (invalid URLs, non-existent codes).
