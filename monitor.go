package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Monitor struct {
	logFilePath           string
	mu                    sync.RWMutex
	lastTimestamps        map[string]time.Time
	IsSessionWorkerClosed map[string]bool
	metrics               Metrics
}

func NewMonitor(logFilePath string) *Monitor {
	return &Monitor{
		logFilePath:           logFilePath,
		lastTimestamps:        map[string]time.Time{},
		IsSessionWorkerClosed: map[string]bool{},
	}
}

type TailReader struct {
	mu         sync.Mutex
	fileName   string
	currentPos int64
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewTailReader(ctx context.Context, fileName string) (*TailReader, error) {
	cctx, cancel := context.WithCancel(ctx)
	return &TailReader{
		fileName: fileName,
		ctx:      cctx,
		cancel:   cancel,
	}, nil
}

func (r *TailReader) Close() error {
	r.cancel()
	return nil
}

func (r *TailReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		default:
		}
		stats, err := os.Stat(r.fileName)
		if err != nil {
			return 0, err
		}
		if stats.Size() <= r.currentPos {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		file, err := os.Open(r.fileName)
		if err != nil {
			return 0, err
		}
		_, err = file.Seek(r.currentPos, io.SeekStart)
		if err != nil {
			return 0, err
		}
		n, err := file.Read(p)
		if err != nil {
			return 0, err
		}
		r.currentPos += int64(n)
		return n, nil
	}
}

func (m *Monitor) Run(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var err error
	for {
		select {
		case <-timeoutCtx.Done():
			if err != nil {
				return err
			}
			return timeoutCtx.Err()
		default:

		}
		_, err = os.Stat(m.logFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		break
	}
	reader, err := NewTailReader(ctx, m.logFilePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	return m.RunWithReader(ctx, reader)
}

func (m *Monitor) RunWithReader(ctx context.Context, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := scanner.Text()
		var e LogEntry
		ok, err := e.Parse(line)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		m.mark(e)
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}
	return nil
}

func (m *Monitor) mark(e LogEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastTimestamps[e.DocumentID] = e.Timestamp
	if e.IsSessionWorkerClosed() {
		m.IsSessionWorkerClosed[e.DocumentID] = true
	}
	var activeConnections, TotalConnections int
	var lastTimestamp time.Time
	for documentID, t := range m.lastTimestamps {
		if t.After(lastTimestamp) {
			lastTimestamp = t
		}
		TotalConnections++
		if !m.IsSessionWorkerClosed[documentID] {
			activeConnections++
		}
	}
	m.metrics = Metrics{
		ActiveConnections: activeConnections,
		TotalConnections:  TotalConnections,
		LastTimestamp:     lastTimestamp,
	}
}

type Metrics struct {
	ActiveConnections int
	TotalConnections  int
	LastTimestamp     time.Time
}

func (m *Monitor) Metrics() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.metrics
}

type LogEntry struct {
	Timestamp  time.Time
	LogLevel   string
	DocumentID string
	Message    string
}

var logEntryRegex = regexp.MustCompile(`^(?P<Timestamp>\S+ \S+) (?P<LogLevel>\S+) \[ssm-session-worker\] \[(?P<DocumentID>\S+)\] (?P<Extra>\[.*\] )?(?P<Message>.*)$`)

func (e *LogEntry) Parse(line string) (bool, error) {
	matches := logEntryRegex.FindStringSubmatch(line)
	if matches == nil {
		return false, nil
	}
	for i, name := range logEntryRegex.SubexpNames() {
		switch name {
		case "Timestamp":
			t, err := time.Parse("2006-01-02 15:04:05", matches[i])
			if err != nil {
				return false, err
			}
			e.Timestamp = t
		case "LogLevel":
			e.LogLevel = matches[i]
		case "DocumentID":
			e.DocumentID = matches[i]
		case "Message":
			e.Message = matches[i]
		}
	}
	return true, nil
}

func (e LogEntry) IsSessionWorkerClosed() bool {
	return strings.EqualFold(e.LogLevel, "INFO") && strings.Contains(strings.ToLower(e.Message), "session worker closed")
}
