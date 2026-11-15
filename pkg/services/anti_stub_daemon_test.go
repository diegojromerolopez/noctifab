package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAntiStubDaemon(t *testing.T) {
	t.Run("detects C socket accept immediately sending HTTP 200 without recv", func(t *testing.T) {
		lines := []string{
			"while (1) {",
			"    int client_fd = accept(server_fd, NULL, NULL);",
			"    if (client_fd < 0) continue;",
			"    send(client_fd, \"HTTP/1.1 200 OK\\r\\nContent-Length: 0\\r\\n\\r\\n\", 38, 0);",
			"    close(client_fd);",
			"}",
		}
		violations := checkDaemonSocketViolations("src/t4.c", lines)
		require.Len(t, violations, 1)
		assert.Equal(t, "hardcoded_unparsing_server_stub", violations[0].Rule)
		assert.Equal(t, 4, violations[0].Line)
		assert.Contains(t, violations[0].Snippet, "HTTP/1.1 200 OK")
	})

	t.Run("passes C socket when recv is called before send", func(t *testing.T) {
		lines := []string{
			"while (1) {",
			"    int client_fd = accept(server_fd, NULL, NULL);",
			"    char buf[1024];",
			"    ssize_t n = recv(client_fd, buf, sizeof(buf)-1, 0);",
			"    if (n > 0) {",
			"        send(client_fd, \"HTTP/1.1 200 OK\\r\\n\\r\\n\", 19, 0);",
			"    }",
			"    close(client_fd);",
			"}",
		}
		violations := checkDaemonSocketViolations("src/t4.c", lines)
		assert.Empty(t, violations)
	})

	t.Run("detects Python socket accept immediately sending PONG without recv", func(t *testing.T) {
		lines := []string{
			"while True:",
			"    conn, addr = s.accept()",
			"    conn.sendall(b\"+PONG\\r\\n\")",
			"    conn.close()",
		}
		violations := checkDaemonSocketViolations("src/pyedis/server.py", lines)
		require.Len(t, violations, 1)
		assert.Equal(t, "hardcoded_unparsing_server_stub", violations[0].Rule)
		assert.Equal(t, 3, violations[0].Line)
	})

	t.Run("passes Python socket when conn.recv is called", func(t *testing.T) {
		lines := []string{
			"while True:",
			"    conn, addr = s.accept()",
			"    data = conn.recv(1024)",
			"    if b\"PING\" in data:",
			"        conn.sendall(b\"+PONG\\r\\n\")",
			"    conn.close()",
		}
		violations := checkDaemonSocketViolations("src/pyedis/server.py", lines)
		assert.Empty(t, violations)
	})

	t.Run("detects Go socket accept immediately writing HTTP 200 without Read", func(t *testing.T) {
		lines := []string{
			"for {",
			"    conn, err := l.Accept()",
			"    if err != nil { continue }",
			"    conn.Write([]byte(\"HTTP/1.1 200 OK\\r\\n\\r\\n\"))",
			"    conn.Close()",
			"}",
		}
		violations := checkDaemonSocketViolations("pkg/server/main.go", lines)
		require.Len(t, violations, 1)
		assert.Equal(t, "hardcoded_unparsing_server_stub", violations[0].Rule)
	})

	t.Run("ignores test files", func(t *testing.T) {
		lines := []string{
			"int client_fd = accept(server_fd, NULL, NULL);",
			"send(client_fd, \"HTTP/1.1 200 OK\\r\\n\\r\\n\", 19, 0);",
		}
		violations := checkDaemonSocketViolations("tests/e2e/test_server.c", lines)
		assert.Empty(t, violations)
	})
}
