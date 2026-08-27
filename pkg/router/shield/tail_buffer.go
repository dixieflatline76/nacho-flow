package shield

import (
	"sync"
)

var defaultTailBufferPool = sync.Pool{
	New: func() interface{} {
		return NewTailBuffer(256)
	},
}

// GetTailBuffer retrieves a 256-byte TailBuffer from the pool.
func GetTailBuffer() *TailBuffer {
	tb := defaultTailBufferPool.Get().(*TailBuffer)
	tb.Reset()
	return tb
}

// PutTailBuffer returns a TailBuffer to the pool.
func PutTailBuffer(tb *TailBuffer) {
	if tb == nil {
		return
	}
	tb.Reset()
	defaultTailBufferPool.Put(tb)
}

// TailBuffer maintains a fixed-size ring of the trailing bytes of a stream.
type TailBuffer struct {
	buf      []byte
	capacity int
	size     int
	head     int
}

// NewTailBuffer creates a circular TailBuffer with a specific capacity.
func NewTailBuffer(capacity int) *TailBuffer {
	if capacity <= 0 {
		capacity = 256
	}
	return &TailBuffer{
		buf:      make([]byte, capacity),
		capacity: capacity,
	}
}

// Append writes new chunk data into the circular sliding buffer.
func (tb *TailBuffer) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	for _, b := range data {
		tb.buf[tb.head] = b
		tb.head = (tb.head + 1) % tb.capacity
		if tb.size < tb.capacity {
			tb.size++
		}
	}
}

// Bytes returns a contiguous linear slice of the buffered trailing bytes.
func (tb *TailBuffer) Bytes() []byte {
	if tb.size == 0 {
		return nil
	}
	out := make([]byte, tb.size)
	if tb.size < tb.capacity {
		copy(out, tb.buf[:tb.size])
		return out
	}
	// Circular wrap-around: oldest bytes start at tb.head up to capacity, then 0 to tb.head
	n1 := copy(out, tb.buf[tb.head:])
	copy(out[n1:], tb.buf[:tb.head])
	return out
}

// Reset clears the buffer size and head pointer without reallocating.
func (tb *TailBuffer) Reset() {
	tb.size = 0
	tb.head = 0
}
