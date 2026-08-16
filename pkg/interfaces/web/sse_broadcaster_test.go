package web

import (
	"testing"
	"time"
)

func TestSSEBroadcaster_SubscriptionAndReplay(t *testing.T) {
	b := NewSSEBroadcaster(10)

	// Broadcast first event before subscriber connects
	_ = b.Broadcast(EventTaskStateChanged, map[string]string{"task": "init"})

	// Subscribe with lastEventID = 0 (no replay requested)
	ch1 := b.Subscribe(0)
	defer b.Unsubscribe(ch1)

	if b.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", b.ClientCount())
	}

	// Broadcast second event
	_ = b.Broadcast(EventConsensusVote, map[string]string{"result": "pass"})

	select {
	case ev := <-ch1:
		if ev.Type != EventConsensusVote {
			t.Errorf("expected EventConsensusVote, got %s", ev.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}

	// Subscribe second client with lastEventID = 1 (requests replay of event 2)
	ch2 := b.Subscribe(1)
	defer b.Unsubscribe(ch2)

	select {
	case ev := <-ch2:
		if ev.ID != 2 {
			t.Errorf("expected replayed event ID 2, got %d", ev.ID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for replayed event")
	}
}

func TestSSEBroadcaster_NonBlockingDrop(t *testing.T) {
	b := NewSSEBroadcaster(5)
	ch := b.Subscribe(0)
	defer b.Unsubscribe(ch)

	// Broadcast more than channel buffer size (128) without reading
	for i := 0; i < 200; i++ {
		_ = b.Broadcast(EventSystemLog, map[string]int{"seq": i})
	}

	// Broadcaster should not have blocked or crashed
	if b.ClientCount() != 1 {
		t.Errorf("expected 1 client still registered, got %d", b.ClientCount())
	}
}
