package telemetry

import (
	"sync"
	"sync/atomic"
)

// DefaultRingBufferCapacity is the default number of recent turn records kept in memory.
const DefaultRingBufferCapacity = 500

// RingBufferSink implements an in-memory, circular ring buffer sink for recent route turns.
type RingBufferSink struct {
	mu           sync.RWMutex
	records      []TurnRecord
	capacity     int
	head         int
	totalTracked atomic.Int64
}

// NewRingBufferSink initializes a circular ring buffer with the specified capacity.
func NewRingBufferSink(capacity int) *RingBufferSink {
	if capacity <= 0 {
		capacity = DefaultRingBufferCapacity
	}
	return &RingBufferSink{
		records:  make([]TurnRecord, capacity),
		capacity: capacity,
	}
}

// Emit appends a TurnRecord to the circular buffer. Zero GC allocations once capacity is reached.
func (r *RingBufferSink) Emit(record TurnRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records[r.head] = record
	r.head = (r.head + 1) % r.capacity
	r.totalTracked.Add(1)
}

// GetRecent returns the most recent N records in reverse chronological order (newest first).
func (r *RingBufferSink) GetRecent(limit int) []TurnRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := r.totalTracked.Load()
	if total == 0 {
		return []TurnRecord{}
	}

	count := int(total)
	if count > r.capacity {
		count = r.capacity
	}
	if limit > 0 && limit < count {
		count = limit
	}

	result := make([]TurnRecord, count)
	// head points to the next write slot, so the most recent item is at (head - 1)
	for i := 0; i < count; i++ {
		idx := (r.head - 1 - i + r.capacity) % r.capacity
		result[i] = r.records[idx]
	}
	return result
}

// Reset clears all records and resets head and totalTracked to zero.
func (r *RingBufferSink) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records = make([]TurnRecord, r.capacity)
	r.head = 0
	r.totalTracked.Store(0)
}

// TotalTracked returns the cumulative number of records processed over the lifecycle.
func (r *RingBufferSink) TotalTracked() int64 {
	return r.totalTracked.Load()
}

// Capacity returns the maximum buffer capacity.
func (r *RingBufferSink) Capacity() int {
	return r.capacity
}

// Close implements ObservationSink.
func (r *RingBufferSink) Close() error {
	return nil
}
