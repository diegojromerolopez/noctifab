package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FSWatcherConfig holds configuration for the filesystem watcher.
type FSWatcherConfig struct {
	BaseDir      string
	Debounce     time.Duration
	PollInterval time.Duration
	OnStory      func(storyPath string)
}

// FSWatcher detects additions and modifications to specification and story files.
type FSWatcher struct {
	baseDir       string
	debounce      time.Duration
	pollInterval  time.Duration
	onStory       func(storyPath string)
	seenModTimes  map[string]time.Time
	pendingEvents map[string]time.Time
	mu            sync.Mutex
	stopCh        chan struct{}
}

// NewFSWatcher initializes a new FSWatcher.
func NewFSWatcher(cfg FSWatcherConfig) *FSWatcher {
	debounce := cfg.Debounce
	if debounce <= 0 {
		debounce = 1500 * time.Millisecond
	}
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	baseDir := cfg.BaseDir
	if baseDir == "" {
		baseDir = "."
	}

	return &FSWatcher{
		baseDir:       baseDir,
		debounce:      debounce,
		pollInterval:  poll,
		onStory:       cfg.OnStory,
		seenModTimes:  make(map[string]time.Time),
		pendingEvents: make(map[string]time.Time),
		stopCh:        make(chan struct{}),
	}
}

// Start begins the watcher background loop.
func (w *FSWatcher) Start(ctx context.Context) {
	// Initialize seenModTimes with current state so startup files aren't treated as new modifications
	w.scanFiles(false)

	go func() {
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.scanFiles(true)
			}
		}
	}()
}

// Stop terminates the watcher.
func (w *FSWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (w *FSWatcher) scanFiles(notify bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	targets := []string{
		filepath.Join(w.baseDir, "SPEC.md"),
	}

	storiesDir := filepath.Join(w.baseDir, "roadmap", "user-stories")
	if matches, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil {
		for _, match := range matches {
			name := filepath.Base(match)
			if !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "~") && !strings.HasSuffix(name, ".tmp") {
				targets = append(targets, match)
			}
		}
	}

	now := time.Now()

	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			continue
		}

		lastMod, seen := w.seenModTimes[target]
		modTime := info.ModTime()

		if !seen {
			w.seenModTimes[target] = modTime
			if notify {
				w.pendingEvents[target] = now
			}
		} else if modTime.After(lastMod) {
			w.seenModTimes[target] = modTime
			if notify {
				w.pendingEvents[target] = now
			}
		}
	}

	for target, detectedAt := range w.pendingEvents {
		if now.Sub(detectedAt) >= w.debounce {
			delete(w.pendingEvents, target)
			if w.onStory != nil {
				w.onStory(target)
			}
		}
	}
}
