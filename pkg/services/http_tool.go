package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// HTTPCheckRequest holds parameters for testing HTTP connectivity.
type HTTPCheckRequest struct {
	URL                  string
	Method               string
	Headers              map[string]string
	Body                 string
	ExpectedStatus       int
	ExpectedBodyContains string
	Timeout              time.Duration
	WaitDuration         time.Duration
}

// HTTPCheckResult holds the outcome of an HTTP check.
type HTTPCheckResult struct {
	StatusCode int
	Latency    time.Duration
	Headers    map[string]string
	Body       string
	Matched    bool
	Error      string
	Diagnostic string
}

// CheckHTTP performs deterministic HTTP request and endpoint health verification.
func CheckHTTP(ctx context.Context, req HTTPCheckRequest) (*HTTPCheckResult, error) {
	urlStr := strings.TrimSpace(req.URL)
	if urlStr == "" {
		return nil, errors.New("missing or empty url")
	}
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "http://" + urlStr
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	waitDuration := req.WaitDuration
	if waitDuration < 0 {
		waitDuration = 0
	}

	deadline := time.Now().Add(waitDuration)
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	var lastResp *http.Response
	var lastErr error
	var latency time.Duration
	var bodyBytes []byte

	for {
		var reqBody io.Reader
		if len(req.Body) > 0 {
			reqBody = strings.NewReader(req.Body)
		}

		httpReq, err := http.NewRequestWithContext(ctx, method, urlStr, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create http request: %w", err)
		}

		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}

		start := time.Now()
		resp, err := client.Do(httpReq)
		latency = time.Since(start)

		if err == nil {
			lastResp = resp
			bodyBytes, _ = io.ReadAll(io.LimitReader(resp.Body, 65536))
			_ = resp.Body.Close()

			if req.ExpectedStatus == 0 || resp.StatusCode == req.ExpectedStatus {
				break
			}
		}
		lastErr = err

		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	res := &HTTPCheckResult{
		Latency: latency,
		Headers: make(map[string]string),
	}

	if lastResp == nil {
		res.Error = fmt.Sprintf("%v", lastErr)
		if strings.Contains(res.Error, "connection refused") {
			res.Diagnostic = fmt.Sprintf("Endpoint %s is unreachable (connection refused). Ensure server process is up.", urlStr)
		} else if strings.Contains(res.Error, "timeout") || strings.Contains(res.Error, "deadline exceeded") {
			res.Diagnostic = fmt.Sprintf("Request to %s timed out. Ensure the server is responsive.", urlStr)
		} else {
			res.Diagnostic = fmt.Sprintf("HTTP request failed: %v", lastErr)
		}
		return res, nil
	}

	res.StatusCode = lastResp.StatusCode
	for k, v := range lastResp.Header {
		if len(v) > 0 {
			res.Headers[k] = v[0]
		}
	}
	res.Body = string(bodyBytes)

	statusMatched := (req.ExpectedStatus == 0 && res.StatusCode >= 200 && res.StatusCode < 400) || (req.ExpectedStatus != 0 && res.StatusCode == req.ExpectedStatus)
	bodyMatched := true
	if req.ExpectedBodyContains != "" {
		bodyMatched = strings.Contains(res.Body, req.ExpectedBodyContains)
	}
	res.Matched = statusMatched && bodyMatched

	if !statusMatched {
		res.Diagnostic = fmt.Sprintf("Status code mismatch: expected %d, got %d", req.ExpectedStatus, res.StatusCode)
	} else if !bodyMatched {
		res.Diagnostic = fmt.Sprintf("Body did not contain expected substring %q", req.ExpectedBodyContains)
	}

	return res, nil
}

// CheckHTTPTool implements check_http for LLM agent HTTP endpoint testing.
type CheckHTTPTool struct{}

// Name returns the unique tool identifier.
func (t *CheckHTTPTool) Name() string { return "check_http" }

// Description returns LLM documentation for check_http.
func (t *CheckHTTPTool) Description() string {
	return "check_http tests an HTTP/HTTPS endpoint. Arguments: url (string, required, e.g. 'http://127.0.0.1:8080/health'), method (string, optional, default 'GET'), headers (object, optional), body (string, optional), expected_status (number, optional, default 200), expected_body_contains (string, optional), timeout_seconds (number, optional, default 5), wait_seconds (number, optional, polls until healthy)."
}

// Execute performs HTTP endpoint testing.
func (t *CheckHTTPTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	urlStr, _ := args["url"].(string)
	if strings.TrimSpace(urlStr) == "" {
		return "", errors.New("missing or empty 'url' argument")
	}

	method, _ := args["method"].(string)
	bodyStr, _ := args["body"].(string)
	expectedBody, _ := args["expected_body_contains"].(string)
	expectedStatus := extractNetworkInt(args["expected_status"], 0)
	timeoutSec := extractNetworkFloat(args["timeout_seconds"], 5.0)
	waitSec := extractNetworkFloat(args["wait_seconds"], 0.0)

	headers := make(map[string]string)
	if hMap, ok := args["headers"].(map[string]any); ok {
		for k, v := range hMap {
			headers[k] = fmt.Sprintf("%v", v)
		}
	} else if hMapStr, ok := args["headers"].(map[string]string); ok {
		headers = hMapStr
	}

	req := HTTPCheckRequest{
		URL:                  urlStr,
		Method:               method,
		Headers:              headers,
		Body:                 bodyStr,
		ExpectedStatus:       expectedStatus,
		ExpectedBodyContains: expectedBody,
		Timeout:              time.Duration(timeoutSec * float64(time.Second)),
		WaitDuration:         time.Duration(waitSec * float64(time.Second)),
	}

	res, err := CheckHTTP(ctx, req)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if res.StatusCode > 0 {
		sb.WriteString("Status: HTTP ")
		sb.WriteString(strconv.Itoa(res.StatusCode))
		sb.WriteString("\nURL: ")
		sb.WriteString(urlStr)
		sb.WriteString("\nLatency: ")
		sb.WriteString(res.Latency.String())
		sb.WriteString("\n")
		if res.Matched {
			sb.WriteString("Assertion: PASSED\n")
		} else {
			sb.WriteString("Assertion: FAILED (")
			sb.WriteString(res.Diagnostic)
			sb.WriteString(")\n")
		}
		if len(res.Body) > 0 {
			sb.WriteString("Body: ")
			if len(res.Body) > 1024 {
				sb.WriteString(res.Body[:1024])
				sb.WriteString("... [truncated]\n")
			} else {
				sb.WriteString(res.Body)
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("Status: UNREACHABLE\n")
		sb.WriteString("URL: ")
		sb.WriteString(urlStr)
		sb.WriteString("\nError: ")
		sb.WriteString(res.Error)
		sb.WriteString("\nDiagnostic: ")
		sb.WriteString(res.Diagnostic)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func extractNetworkFloat(val any, defaultVal float64) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func extractNetworkInt(val any, defaultVal int) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
