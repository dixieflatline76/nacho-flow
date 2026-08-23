package telemetry

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType represents the SSE event category.
type EventType string

const (
	EventRouteCompleted      EventType = "route_completed"
	EventCircuitStateChanged EventType = "circuit_state_changed"
	EventConfigUpdated       EventType = "config_updated"
)

// Event represents an individual SSE event broadcast to connected clients.
type Event struct {
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// CircuitEventData describes a circuit breaker transition event.
type CircuitEventData struct {
	Provider string `json:"provider"`
	State    string `json:"state"`
	Failures int    `json:"failures"`
}

// ConfigEventData describes a configuration reload event.
type ConfigEventData struct {
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// EventBroker manages thread-safe subscriber channels for Server-Sent Events (SSE).
type EventBroker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	closed      bool
}

// NewEventBroker initializes a new SSE event broker.
func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new subscriber channel with a buffer.
func (b *EventBroker) Subscribe(bufSize int) chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bufSize <= 0 {
		bufSize = 100
	}
	ch := make(chan Event, bufSize)
	if b.closed {
		close(ch)
		return ch
	}
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes and closes a subscriber channel.
func (b *EventBroker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[ch]; exists {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// Publish broadcasts an event to all active subscribers. Non-blocking drop on slow clients.
func (b *EventBroker) Publish(evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
			// Non-blocking: skip if client buffer is saturated
		}
	}
}

// PublishJSON helper encodes payload data into an Event.
func (b *EventBroker) PublishJSON(evtType EventType, payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	b.Publish(Event{
		Type:      evtType,
		Timestamp: time.Now().UTC(),
		Data:      dataBytes,
	})
	return nil
}

// Emit implements ObservationSink to automatically broadcast route completions.
func (b *EventBroker) Emit(record TurnRecord) {
	_ = b.PublishJSON(EventRouteCompleted, record)
}

// Close closes the broker and terminates all subscriber channels.
func (b *EventBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true
	for ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, ch)
	}
	return nil
}

// SubscriberCount returns the current count of connected SSE subscribers.
func (b *EventBroker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
