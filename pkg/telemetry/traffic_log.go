package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// TrafficLogger writes streaming TurnRecord observations to a JSONL file asynchronously.
type TrafficLogger struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	queue    chan TurnRecord
	closeReq chan struct{}
	flushReq chan chan struct{}
	done     chan struct{}
	closed   atomic.Bool
}

// NewTrafficLogger creates a new TrafficLogger targeting the specified path.
func NewTrafficLogger(filePath string, bufferSize int) (*TrafficLogger, error) {
	if filePath == "" {
		filePath = filepath.Join(contract.ResolveLogDir(""), contract.DefaultTrafficLogFileName)
	}

	if bufferSize <= 0 {
		bufferSize = 5000
	}

	filePath = filepath.Clean(filePath)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create traffic log directory: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open traffic log file: %w", err)
	}

	tl := &TrafficLogger{
		filePath: filePath,
		file:     file,
		writer:   bufio.NewWriterSize(file, 64*1024), // 64KB write buffer
		queue:    make(chan TurnRecord, bufferSize),
		closeReq: make(chan struct{}),
		flushReq: make(chan chan struct{}),
		done:     make(chan struct{}),
	}

	go tl.worker()
	return tl, nil
}

// FilePath returns the underlying destination file path.
func (tl *TrafficLogger) FilePath() string {
	return tl.filePath
}

func (tl *TrafficLogger) worker() {
	defer close(tl.done)

	flushTicker := time.NewTicker(2 * time.Second)
	defer flushTicker.Stop()

	for {
		select {
		case record := <-tl.queue:
			tl.writeRecord(record)
		case <-flushTicker.C:
			tl.flush()
		case done := <-tl.flushReq:
			for {
				select {
				case record := <-tl.queue:
					tl.writeRecord(record)
				default:
					goto flushed
				}
			}
		flushed:
			tl.flush()
			close(done)
		case <-tl.closeReq:
			// Drain remaining records in queue before terminating
			for {
				select {
				case record := <-tl.queue:
					tl.writeRecord(record)
				default:
					tl.flush()
					return
				}
			}
		}
	}
}

func (tl *TrafficLogger) writeRecord(record TurnRecord) {
	data, err := json.Marshal(record)
	if err == nil {
		_, _ = tl.writer.Write(data)
		_ = tl.writer.WriteByte('\n')
	}
}

func (tl *TrafficLogger) flush() {
	if tl.writer != nil {
		_ = tl.writer.Flush()
	}
	if tl.file != nil {
		_ = tl.file.Sync()
	}
}

// Flush synchronizes all currently queued TurnRecord entries to disk.
func (tl *TrafficLogger) Flush() {
	if tl.closed.Load() {
		return
	}
	done := make(chan struct{})
	select {
	case tl.flushReq <- done:
		<-done
	case <-time.After(2 * time.Second):
	}
}

// Emit sends a TurnRecord to the non-blocking asynchronous write queue.
// This call is completely lock-free and thread-safe.
func (tl *TrafficLogger) Emit(record TurnRecord) {
	if tl.closed.Load() {
		return
	}

	select {
	case tl.queue <- record:
	default:
		// Drop non-blocking if queue is saturated under extreme overload
	}
}

// Close flushes buffered writes and closes the file.
func (tl *TrafficLogger) Close() error {
	if tl.closed.Swap(true) {
		return nil
	}
	close(tl.closeReq)
	<-tl.done

	if tl.file != nil {
		return tl.file.Close()
	}
	return nil
}

// ReadRecords reads historical TurnRecord entries from the JSONL file.
func ReadRecords(filePath string, limit int) ([]TurnRecord, error) {
	filePath = filepath.Clean(filePath)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TurnRecord{}, nil
		}
		return nil, fmt.Errorf("failed to open traffic log for reading: %w", err)
	}
	defer func() { _ = file.Close() }()

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

// ReadCompleteSessions reads historical TurnRecord entries preserving full session trajectories.
// If maxSessions > 0, it returns all turns belonging to the most recent maxSessions complete sessions.
// If maxSessions <= 0, it returns all turns across all recorded sessions without arbitrary truncation.
// Lines that are corrupt or partially written (e.g. from concurrent writes at EOF) are safely skipped.
func ReadCompleteSessions(filePath string, maxSessions int) ([]TurnRecord, error) {
	filePath = filepath.Clean(filePath)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TurnRecord{}, nil
		}
		return nil, fmt.Errorf("failed to open traffic log for reading: %w", err)
	}
	defer func() { _ = file.Close() }()

	sessions := make(map[string][]TurnRecord)
	sessionOrder := make([]string, 0)
	sessionIndex := make(map[string]int)

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024) // up to 10MB lines

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r TurnRecord
		if err := json.Unmarshal(line, &r); err != nil {
			// Skip corrupted or incomplete trailing lines from concurrent appends
			continue
		}

		key := r.SessionID
		if r.RootPromptHash != 0 {
			key = fmt.Sprintf("%s:%d", r.SessionID, r.RootPromptHash)
		}
		if key == "" {
			key = fmt.Sprintf("anon_%d_%s", lineNum, r.RequestID)
		}

		if _, exists := sessionIndex[key]; !exists {
			sessionIndex[key] = len(sessionOrder)
			sessionOrder = append(sessionOrder, key)

			// If maxSessions is set and exceeded, evict the oldest session
			if maxSessions > 0 && len(sessionOrder) > maxSessions {
				oldest := sessionOrder[0]
				sessionOrder = sessionOrder[1:]
				delete(sessions, oldest)
				delete(sessionIndex, oldest)
				for i, k := range sessionOrder {
					sessionIndex[k] = i
				}
			}
		}

		sessions[key] = append(sessions[key], r)
	}

	totalRecords := 0
	for _, k := range sessionOrder {
		totalRecords += len(sessions[k])
	}

	records := make([]TurnRecord, 0, totalRecords)
	for _, k := range sessionOrder {
		records = append(records, sessions[k]...)
	}

	return records, scanner.Err()
}
