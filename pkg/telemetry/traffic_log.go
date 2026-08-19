package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrafficLogger writes streaming TurnRecord observations to a JSONL file asynchronously.
type TrafficLogger struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	queue    chan TurnRecord
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
}

// NewTrafficLogger creates a new TrafficLogger targeting the specified path.
func NewTrafficLogger(filePath string, bufferSize int) (*TrafficLogger, error) {
	if filePath == "" {
		filePath = filepath.Join("logs", "traffic.jsonl")
	}

	if bufferSize <= 0 {
		bufferSize = 5000
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create traffic log directory: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open traffic log file: %w", err)
	}

	tl := &TrafficLogger{
		filePath: filePath,
		file:     file,
		writer:   bufio.NewWriterSize(file, 64*1024), // 64KB write buffer
		queue:    make(chan TurnRecord, bufferSize),
		done:     make(chan struct{}),
	}

	go tl.worker()
	return tl, nil
}

func (tl *TrafficLogger) worker() {
	defer close(tl.done)

	flushTicker := time.NewTicker(2 * time.Second)
	defer flushTicker.Stop()

	for {
		select {
		case record, ok := <-tl.queue:
			if !ok {
				tl.flush()
				return
			}
			data, err := json.Marshal(record)
			if err == nil {
				tl.mu.Lock()
				_, _ = tl.writer.Write(data)
				_ = tl.writer.WriteByte('\n')
				tl.mu.Unlock()
			}
		case <-flushTicker.C:
			tl.flush()
		}
	}
}

func (tl *TrafficLogger) flush() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.writer != nil {
		_ = tl.writer.Flush()
	}
	if tl.file != nil {
		_ = tl.file.Sync()
	}
}

// Emit sends a TurnRecord to the non-blocking asynchronous write queue.
func (tl *TrafficLogger) Emit(record TurnRecord) {
	tl.mu.Lock()
	if tl.closed {
		tl.mu.Unlock()
		return
	}
	tl.mu.Unlock()

	select {
	case tl.queue <- record:
	default:
		// Drop non-blocking if queue is saturated under extreme overload
	}
}

// Close flushes buffered writes and closes the file.
func (tl *TrafficLogger) Close() error {
	tl.mu.Lock()
	if tl.closed {
		tl.mu.Unlock()
		return nil
	}
	tl.closed = true
	close(tl.queue)
	tl.mu.Unlock()

	<-tl.done

	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.file != nil {
		return tl.file.Close()
	}
	return nil
}

// ReadRecords reads historical TurnRecord entries from the JSONL file.
func ReadRecords(filePath string, limit int) ([]TurnRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TurnRecord{}, nil
		}
		return nil, fmt.Errorf("failed to open traffic log for reading: %w", err)
	}
	defer file.Close()

	var records []TurnRecord
	scanner := bufio.NewScanner(file)
	// Support large line buffers up to 1MB
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r TurnRecord
		if err := json.Unmarshal(line, &r); err == nil {
			records = append(records, r)
		}
	}

	if limit > 0 && len(records) > limit {
		// Return the most recent records
		records = records[len(records)-limit:]
	}

	return records, scanner.Err()
}
