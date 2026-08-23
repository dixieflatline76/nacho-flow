package telemetry

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestEventBroker_SubscribePublishUnsubscribe(t *testing.T) {
	broker := NewEventBroker()
	defer func() { _ = broker.Close() }()

	if broker.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers initially")
	}

	ch1 := broker.Subscribe(10)
	ch2 := broker.Subscribe(10)

	if broker.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", broker.SubscriberCount())
	}

	// Publish an event
	err := broker.PublishJSON(EventCircuitStateChanged, CircuitEventData{
		Provider: "ollama",
		State:    "open",
		Failures: 2,
	})
	if err != nil {
		t.Fatalf("PublishJSON err: %v", err)
	}

	// Verify both channels received the event
	select {
	case evt := <-ch1:
		if evt.Type != EventCircuitStateChanged {
			t.Errorf("expected EventCircuitStateChanged, got %s", evt.Type)
		}
		var data CircuitEventData
		if err := json.Unmarshal(evt.Data, &data); err != nil || data.Provider != "ollama" || data.State != "open" {
			t.Fatalf("unexpected event data: %+v", data)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event on ch1")
	}

	select {
	case evt := <-ch2:
		if evt.Type != EventCircuitStateChanged {
			t.Errorf("expected EventCircuitStateChanged, got %s", evt.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event on ch2")
	}

	// Emit via ObservationSink interface
	broker.Emit(TurnRecord{
		RequestID: "req-test-sse",
		Tokens:    1234,
	})

	select {
	case evt := <-ch1:
		if evt.Type != EventRouteCompleted {
			t.Errorf("expected EventRouteCompleted, got %s", evt.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for route_completed on ch1")
	}

	// Unsubscribe ch1
	broker.Unsubscribe(ch1)
	if broker.SubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", broker.SubscriberCount())
	}

	// Verify closed channel
	_, ok := <-ch1
	if ok {
		t.Errorf("expected ch1 to be closed")
	}
}

func TestEventBroker_ConcurrentPublishSubscribe(t *testing.T) {
	broker := NewEventBroker()
	var wg sync.WaitGroup

	// Start 10 subscriber goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := broker.Subscribe(100)
			for j := 0; j < 20; j++ {
				select {
				case <-ch:
				case <-time.After(50 * time.Millisecond):
				}
			}
			broker.Unsubscribe(ch)
		}()
	}

	// Start 5 publisher goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(pid int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = broker.PublishJSON(EventConfigUpdated, ConfigEventData{
					Timestamp: time.Now().Format(time.RFC3339),
					Version:   "0.6.0",
				})
			}
		}(i)
	}

	wg.Wait()
	_ = broker.Close()
}

func TestEventBroker_EdgeCasesAndClose(t *testing.T) {
	broker := NewEventBroker()

	// 1. Subscribe with 0 bufferSize defaults to 100
	ch := broker.Subscribe(0)
	if ch == nil {
		t.Fatalf("expected non-nil channel")
	}

	// 2. PublishJSON serialization failure
	err := broker.PublishJSON("bad_event", make(chan int))
	if err == nil {
		t.Errorf("expected error for unmarshallable data, got nil")
	}

	// 3. Double Close
	if err := broker.Close(); err != nil {
		t.Errorf("first close failed: %v", err)
	}
	if err := broker.Close(); err != nil {
		t.Errorf("second close should be idempotent: %v", err)
	}

	// 4. Subscribe and Publish after close
	closedCh := broker.Subscribe(10)
	if _, ok := <-closedCh; ok {
		t.Errorf("expected channel to be closed immediately when subscribing to closed broker")
	}

	broker.Publish(Event{Type: "test"})
	broker.Unsubscribe(ch)
}

