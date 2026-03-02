package greywallapi

import (
	"testing"
	"time"
)

func TestEventBusSubscribePublish(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe(10)
	defer bus.Unsubscribe(ch)

	bus.Publish(Event{Type: EventPendingCreated, Data: "test"})

	select {
	case evt := <-ch:
		if evt.Type != EventPendingCreated {
			t.Errorf("got type %q, want %q", evt.Type, EventPendingCreated)
		}
		if evt.Data != "test" {
			t.Errorf("got data %v, want 'test'", evt.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1 := bus.Subscribe(10)
	ch2 := bus.Subscribe(10)
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	bus.Publish(Event{Type: EventPendingCreated, Data: "multi"})

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.Type != EventPendingCreated {
				t.Errorf("subscriber %d: got type %q", i, evt.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe(10)
	bus.Unsubscribe(ch)

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel after unsubscribe")
	}

	// Publishing after unsubscribe should not panic
	bus.Publish(Event{Type: EventPendingCreated})
}

func TestEventBusDropSlowSubscriber(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe(1) // buffer of 1
	defer bus.Unsubscribe(ch)

	// Fill the buffer
	bus.Publish(Event{Type: "first"})
	// This should be dropped (non-blocking)
	bus.Publish(Event{Type: "second"})

	evt := <-ch
	if evt.Type != "first" {
		t.Errorf("expected 'first', got %q", evt.Type)
	}

	// Channel should be empty now
	select {
	case <-ch:
		t.Error("expected empty channel after consuming one event")
	default:
		// OK
	}
}

func TestEventJSON(t *testing.T) {
	evt := Event{Type: EventPendingCreated, Data: map[string]string{"key": "value"}}
	b := evt.JSON()
	if len(b) == 0 {
		t.Error("expected non-empty JSON")
	}
}
