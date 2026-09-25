package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	lineColRE    = regexp.MustCompile(`:\d+:\d+:?`)
	hexAddressRE = regexp.MustCompile(`0x[0-9a-fA-F]{4,16}`)
	timestampRE  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?`)
	tempPathRE   = regexp.MustCompile(`/(?:tmp|var/folders)/[a-zA-Z0-9_\-\.\/]+`)
	durationRE   = regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ms|µs|ns|s|m)\b`)
	spacesRE     = regexp.MustCompile(`\s+`)
)

// ErrorFingerprinter normalizes compiler, runtime, and test errors to produce deterministic
// hash signatures, preventing agents from entering infinite sycophantic repair loops.
type ErrorFingerprinter struct {
	mu            sync.Mutex
	seenHashes    map[string]int
	lastHash      string
	maxDuplicates int
}

// NewErrorFingerprinter creates an ErrorFingerprinter instance.
func NewErrorFingerprinter(maxDuplicates ...int) *ErrorFingerprinter {
	max := 1
	if len(maxDuplicates) > 0 && maxDuplicates[0] > 0 {
		max = maxDuplicates[0]
	}
	return &ErrorFingerprinter{
		seenHashes:    make(map[string]int),
		maxDuplicates: max,
	}
}

// NormalizeError removes variable timestamps, line numbers, memory pointers, and temp paths.
func (f *ErrorFingerprinter) NormalizeError(raw string) string {
	res := lineColRE.ReplaceAllString(raw, ":LINE:COL:")
	res = hexAddressRE.ReplaceAllString(res, "0xADDR")
	res = timestampRE.ReplaceAllString(res, "TIMESTAMP")
	res = tempPathRE.ReplaceAllString(res, "/tmp/DIR")
	res = durationRE.ReplaceAllString(res, "DURATION")
	res = spacesRE.ReplaceAllString(res, " ")
	return strings.TrimSpace(res)
}

// Fingerprint computes the SHA256 hexadecimal hash of the normalized error string.
func (f *ErrorFingerprinter) Fingerprint(raw string) string {
	normalized := f.NormalizeError(raw)
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// RecordAndCheckLoop records the error and determines whether an identical failure signature
// has occurred consecutively or exceeded the allowed duplicate threshold.
func (f *ErrorFingerprinter) RecordAndCheckLoop(raw string) (isLoop bool, hash string, diagnostic string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	hash = f.Fingerprint(raw)
	f.seenHashes[hash]++
	count := f.seenHashes[hash]

	if count > f.maxDuplicates || (f.lastHash != "" && f.lastHash == hash) {
		diagnostic = fmt.Sprintf("sycophantic error loop detected: identical normalized error fingerprint %s was produced %d time(s)", hash[:12], count)
		return true, hash, diagnostic
	}

	f.lastHash = hash
	return false, hash, ""
}

// Reset clears the recorded error history.
func (f *ErrorFingerprinter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seenHashes = make(map[string]int)
	f.lastHash = ""
}
