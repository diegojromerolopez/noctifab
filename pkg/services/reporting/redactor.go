package reporting

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9\-\._~\+\/]+=*`),
	regexp.MustCompile(`(?i)(basic\s+)[A-Za-z0-9\-\._~\+\/]+=*`),
	regexp.MustCompile(`(?i)(api[_\-]?key|token|password|secret|auth)\s*[:=]\s*["']?[A-Za-z0-9\-_.~+%/]{6,}["']?`),
	regexp.MustCompile(`\b(sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{20,}|glpat-[A-Za-z0-9]{20,})\b`),
}

type Redactor struct{}

func NewRedactor() *Redactor {
	return &Redactor{}
}

func (r *Redactor) Redact(text string) string {
	if text == "" {
		return ""
	}
	res := text
	for _, pat := range secretPatterns {
		res = pat.ReplaceAllString(res, "${1}[REDACTED_SECRET]")
	}
	return res
}

func (r *Redactor) BoundedRedact(text string, maxBytes int) string {
	redacted := r.Redact(text)
	if len(redacted) <= maxBytes {
		return redacted
	}

	origBytes := len(redacted)
	truncated := redacted[:maxBytes]
	for !utf8.ValidString(truncated) {
		maxBytes--
		truncated = redacted[:maxBytes]
	}
	return fmt.Sprintf("%s… [truncated, original bytes=%d]", truncated, origBytes)
}
