package telemetry

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRingBufferSink_BasicAndWrap(t *testing.T) {
	rb := NewRingBufferSink(5)

	if rb.Capacity() != 5 {
		t.Fatalf("expected capacity 5, got %d", rb.Capacity())
	}
	if len(rb.GetRecent(10)) != 0 {
		t.Fatalf("expected empty buffer initially")
	}

	// Insert 3 items
	for i := 1; i <= 3; i++ {
		rb.Emit(TurnRecord{
			RequestID: fmt.Sprintf("req-%d", i),
			Tokens:    i * 100,
		})
	}

	if rb.TotalTracked() != 3 {
		t.Fatalf("expected total 3, got %d", rb.TotalTracked())
	}

	recent := rb.GetRecent(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 items, got %d", len(recent))
	}
	if recent[0].RequestID != "req-3" || recent[1].RequestID != "req-2" || recent[2].RequestID != "req-1" {
		t.Fatalf("unexpected order: %+v", recent)
	}

	// Insert 4 more items to force wrap-around (total 7 items in capacity 5)
	for i := 4; i <= 7; i++ {
		rb.Emit(TurnRecord{
			RequestID: fmt.Sprintf("req-%d", i),
			Tokens:    i * 100,
		})
	}

	if rb.TotalTracked() != 7 {
		t.Fatalf("expected total 7, got %d", rb.TotalTracked())
	}

	recent5 := rb.GetRecent(10)
	if len(recent5) != 5 {
		t.Fatalf("expected 5 items (capacity limit), got %d", len(recent5))
	}
	// Expected newest to oldest: req-7, req-6, req-5, req-4, req-3
	expectedIDs := []string{"req-7", "req-6", "req-5", "req-4", "req-3"}
	for i, exp := range expectedIDs {
		if recent5[i].RequestID != exp {
			t.Errorf("at index %d: expected %s, got %s", i, exp, recent5[i].RequestID)
		}
	}

	// Test limit smaller than count
	recent2 := rb.GetRecent(2)
	if len(recent2) != 2 || recent2[0].RequestID != "req-7" || recent2[1].RequestID != "req-6" {
		t.Fatalf("expected limit 2 to yield req-7, req-6; got %+v", recent2)
	}

	if err := rb.Close(); err != nil {
		t.Fatalf("unexpected close err: %v", err)
	}
}

func TestRingBufferSink_ConcurrentRace(t *testing.T) {
	rb := NewRingBufferSink(50)
	var wg sync.WaitGroup

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				rb.Emit(TurnRecord{
					RequestID: fmt.Sprintf("goroutine-%d-turn-%d", id, i),
					Timestamp: time.Now(),
					Tokens:    i,
				})
				_ = rb.GetRecent(10)
			}
		}(g)
	}

	wg.Wait()
	if rb.TotalTracked() != 1000 {
		t.Fatalf("expected total 1000, got %d", rb.TotalTracked())
	}
}

func TestRingBufferSink_ZeroCapacityAndLimit(t *testing.T) {
	rb := NewRingBufferSink(0)
	if rb.Capacity() != 500 {
		t.Errorf("expected default capacity 500 when <= 0, got %d", rb.Capacity())
	}
	rb.Emit(TurnRecord{RequestID: "1"})
	if len(rb.GetRecent(0)) != 1 {
		t.Errorf("expected 1 item for limit 0 (fallback to capacity)")
	}
}
