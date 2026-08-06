package services

import "sync"

// defaultBoundedBufferMax is the default cap (1MB) applied when a
// BoundedBuffer is created with a non-positive maximum.
const defaultBoundedBufferMax = 1 << 20

// BoundedBuffer is a concurrency-safe io.Writer that keeps at most `max`
// bytes of the MOST RECENT output written to it (the tail). It is used to
// cap the memory consumed by long-running command output capture, where the
// tail (recent output) matters most, e.g. for test logs.
type BoundedBuffer struct {
	mu        sync.Mutex
	max       int
	buf       []byte
	truncated bool
}

// NewBoundedBuffer creates a BoundedBuffer capped at max bytes. A
// non-positive max defaults to 1MB.
func NewBoundedBuffer(max int) *BoundedBuffer {
	if max <= 0 {
		max = defaultBoundedBufferMax
	}
	return &BoundedBuffer{max: max}
}

// Write appends p, discarding the oldest bytes when the cap is exceeded.
// It always reports the full len(p) as written.
func (b *BoundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.max <= 0 {
		b.max = defaultBoundedBufferMax
	}
	if len(p) >= b.max {
		// The chunk alone exceeds the cap: keep only its tail.
		b.buf = append(b.buf[:0], p[len(p)-b.max:]...)
		if len(p) > b.max {
			b.truncated = true
		}
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	if overflow := len(b.buf) - b.max; overflow > 0 {
		b.buf = b.buf[overflow:]
		b.truncated = true
	}
	return len(p), nil
}

// Bytes returns a copy of the currently retained (tail) bytes.
func (b *BoundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.buf))
	copy(out, b.buf)
	return out
}

// String returns the currently retained (tail) output as a string.
func (b *BoundedBuffer) String() string {
	return string(b.Bytes())
}

// Truncated reports whether any bytes have been discarded.
func (b *BoundedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
