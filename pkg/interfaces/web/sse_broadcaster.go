package web

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType classifies the payload sent over Server-Sent Events.
type EventType string

const (
	EventTaskStateChanged  EventType = "TASK_STATE_CHANGED"
	EventDiffChunkAppended EventType = "DIFF_CHUNK_APPENDED"
	EventConsensusVote     EventType = "CONSENSUS_VOTE"
	EventTokenMetrics      EventType = "TOKEN_METRICS"
	EventSystemLog         EventType = "SYSTEM_LOG"
	EventKeepAlive         EventType = "KEEPALIVE"
)

// SSEEvent represents a single Server-Sent Event frame.
type SSEEvent struct {
	ID        int64           `json:"id"`
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// SSEBroadcaster manages active client streams with circular ring-buffered replay.
type SSEBroadcaster struct {
	mu          sync.RWMutex
	clients     map[chan SSEEvent]bool
	history     []SSEEvent
	historyCap  int
	nextEventID int64
}

// NewSSEBroadcaster creates a thread-safe broadcaster with the given history buffer capacity.
func NewSSEBroadcaster(historyCap int) *SSEBroadcaster {
	if historyCap <= 0 {
		historyCap = 100
	}
	return &SSEBroadcaster{
		clients:    make(map[chan SSEEvent]bool),
		history:    make([]SSEEvent, 0, historyCap),
		historyCap: historyCap,
	}
}

// Subscribe registers a new client channel and replays missed events if lastEventID > 0.
func (b *SSEBroadcaster) Subscribe(lastEventID int64) chan SSEEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan SSEEvent, 128)
	b.clients[ch] = true

	// Replay missed historical events
	if lastEventID > 0 {
		for _, ev := range b.history {
			if ev.ID > lastEventID {
				select {
				case ch <- ev:
				default:
				}
			}
		}
	}
	return ch
}

// Unsubscribe unregisters and closes a client channel.
func (b *SSEBroadcaster) Unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.clients[ch]; exists {
		delete(b.clients, ch)
		close(ch)
	}
}

// Broadcast sends an event non-blockingly to all subscribers and appends to the ring buffer.
func (b *SSEBroadcaster) Broadcast(eventType EventType, data any) error {
	var raw json.RawMessage
	if data != nil {
		bData, err := json.Marshal(data)
		if err != nil {
			return err
		}
		raw = bData
	} else {
		raw = json.RawMessage(`{}`)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextEventID++
	event := SSEEvent{
		ID:        b.nextEventID,
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Payload:   raw,
	}

	// Append to circular buffer
	if len(b.history) >= b.historyCap {
		b.history = b.history[1:]
	}
	b.history = append(b.history, event)

	// Dispatch non-blockingly to all active clients
	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Client buffer full: drop frame to prevent stalling orchestrator
		}
	}

	return nil
}

// ClientCount returns the number of active connected clients.
func (b *SSEBroadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
