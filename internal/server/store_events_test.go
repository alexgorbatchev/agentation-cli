package server

import (
	"fmt"
	"testing"
	"time"
)

func TestStoreSessionSubscriptionAppliesBackpressure(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	store := NewStore()
	session := store.CreateSession("http://example.com", "")

	events, unsubscribe := store.SubscribeSession(session.ID)
	defer unsubscribe()

	const burstCount = 96
	publishDone, publishErr := publishAnnotationsAsync(store, session.ID, burstCount)

	select {
	case <-publishDone:
		t.Fatal("expected publisher to block once subscriber buffer is full")
	case <-time.After(50 * time.Millisecond):
	}

	received := drainEvents(t, events, burstCount, 3*time.Second)
	if received != burstCount {
		t.Fatalf("drained %d events, want %d", received, burstCount)
	}

	waitForPublishCompletion(t, publishDone, publishErr, 2*time.Second)
}

func TestStoreGlobalSubscriptionDeliversBurstWithoutSilentDrop(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	store := NewStore()
	session := store.CreateSession("http://example.com", "")

	events, unsubscribe := store.SubscribeAll()
	defer unsubscribe()

	const burstCount = 96
	publishDone, publishErr := publishAnnotationsAsync(store, session.ID, burstCount)

	time.Sleep(50 * time.Millisecond)
	received := drainEvents(t, events, burstCount, 3*time.Second)
	if received != burstCount {
		t.Fatalf("received %d events, want %d", received, burstCount)
	}

	waitForPublishCompletion(t, publishDone, publishErr, 2*time.Second)
}

func nonBlockingSend(ch chan Event, event Event) {
	select {
	case ch <- event:
	default:
	}
}

func publishAnnotationsAsync(store *Store, sessionID string, count int) (<-chan struct{}, <-chan error) {
	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		defer close(done)
		for i := range count {
			annotation := Annotation{
				Comment:     fmt.Sprintf("annotation-%d", i),
				Element:     "button",
				ElementPath: "body > button",
			}
			if _, ok := store.AddAnnotation(sessionID, annotation); !ok {
				errCh <- fmt.Errorf("AddAnnotation failed at index %d", i)
				return
			}
		}
	}()
	return done, errCh
}

func drainEvents(t *testing.T, events <-chan Event, want int, timeout time.Duration) int {
	t.Helper()
	deadline := time.After(timeout)
	received := 0
	for received < want {
		select {
		case <-events:
			received++
		case <-deadline:
			t.Fatalf("timed out draining events: received=%d want=%d", received, want)
		}
	}
	return received
}

func waitForPublishCompletion(t *testing.T, done <-chan struct{}, errCh <-chan error, timeout time.Duration) {
	t.Helper()
	select {
	case <-done:
		select {
		case err := <-errCh:
			t.Fatalf("publish failed: %v", err)
		default:
		}
	case <-time.After(timeout):
		t.Fatal("publisher did not complete after drain")
	}
}
