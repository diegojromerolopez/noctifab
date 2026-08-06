package services

import (
	"strings"
	"testing"
)

func TestBoundedBuffer(t *testing.T) {
	t.Run("when writes stay under the cap it retains everything", func(t *testing.T) {
		b := NewBoundedBuffer(16)
		_, _ = b.Write([]byte("hello "))
		_, _ = b.Write([]byte("world"))
		if got := b.String(); got != "hello world" {
			t.Errorf("expected 'hello world', got %q", got)
		}
		if b.Truncated() {
			t.Error("expected no truncation")
		}
	})

	t.Run("when writes exceed the cap it keeps only the tail", func(t *testing.T) {
		b := NewBoundedBuffer(8)
		_, _ = b.Write([]byte("0123456789"))
		if got := b.String(); got != "23456789" {
			t.Errorf("expected tail '23456789', got %q", got)
		}
		if !b.Truncated() {
			t.Error("expected truncation flag to be set")
		}
	})

	t.Run("when many small writes overflow the cap it keeps the most recent bytes", func(t *testing.T) {
		b := NewBoundedBuffer(5)
		for _, s := range []string{"aa", "bb", "cc", "dd"} {
			_, _ = b.Write([]byte(s))
		}
		if got := b.String(); got != "bccdd" {
			t.Errorf("expected 'bccdd', got %q", got)
		}
	})

	t.Run("when a single write equals the cap it is fully retained without truncation", func(t *testing.T) {
		b := NewBoundedBuffer(4)
		_, _ = b.Write([]byte("abcd"))
		if got := b.String(); got != "abcd" {
			t.Errorf("expected 'abcd', got %q", got)
		}
		if b.Truncated() {
			t.Error("expected no truncation for exact-cap write")
		}
	})

	t.Run("when created with non-positive max it defaults to 1MB", func(t *testing.T) {
		b := NewBoundedBuffer(0)
		big := strings.Repeat("x", defaultBoundedBufferMax+10)
		_, _ = b.Write([]byte(big))
		if len(b.Bytes()) != defaultBoundedBufferMax {
			t.Errorf("expected retained size %d, got %d", defaultBoundedBufferMax, len(b.Bytes()))
		}
	})

	t.Run("when writing it always reports the full input length as written", func(t *testing.T) {
		b := NewBoundedBuffer(2)
		n, err := b.Write([]byte("abcdef"))
		if err != nil || n != 6 {
			t.Errorf("expected n=6 err=nil, got n=%d err=%v", n, err)
		}
	})
}
