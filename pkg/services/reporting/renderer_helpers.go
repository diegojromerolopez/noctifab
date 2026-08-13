package reporting

import (
	"fmt"
	"strings"
	"time"
)

func (r *Renderer) sanitizeCell(s string) string {
	if s == "" {
		return ""
	}
	s = r.redactor.Redact(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// formatDuration formats milliseconds into human readable unified duration omitting zero units.
// Examples:
//
//	6114ms   -> "6s 114ms"
//	3747003ms -> "1h 2m 27s 3ms"
//	203657ms -> "3m 23s 657ms"
//	0ms      -> "0ms"
func (r *Renderer) formatDuration(ms int64) string {
	if ms <= 0 {
		return "0ms"
	}
	d := time.Duration(ms) * time.Millisecond
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	d -= seconds * time.Second
	millis := d / time.Millisecond

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if millis > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dms", millis))
	}
	return strings.Join(parts, " ")
}

func (r *Renderer) formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	diff := time.Since(t).Truncate(time.Second)
	if diff < 0 {
		diff = 0
	}
	return fmt.Sprintf("%s ago", diff.String())
}
