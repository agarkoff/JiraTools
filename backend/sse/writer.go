package sse

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"jira-tools-web/models"
)

type Writer struct {
	w        http.ResponseWriter
	flusher  http.Flusher
	runID    int
	db       *sql.DB
	lineNum  int
	eventSeq int
	mu       sync.Mutex
}

func NewWriter(w http.ResponseWriter, db *sql.DB, runID int) *Writer {
	flusher, _ := w.(http.Flusher)
	return &Writer{
		w:       w,
		flusher: flusher,
		runID:   runID,
		db:      db,
	}
}

func SetupSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func (s *Writer) sendEvent(event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *Writer) SendStarted() {
	s.sendEvent("started", map[string]int{"run_id": s.runID})
}

func (s *Writer) SendCompleted() {
	s.sendEvent("completed", map[string]int{"run_id": s.runID})
}

func (s *Writer) SendError(msg string) {
	s.sendEvent("error", map[string]string{"message": msg})
}

func (s *Writer) SendProgress(current, total int) {
	s.sendEvent("progress", map[string]int{"current": current, "total": total})
}

// Write implements io.Writer so tabwriter can write to it
func (s *Writer) Write(p []byte) (int, error) {
	text := string(p)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		s.writeLine(line)
	}
	return len(p), nil
}

// Printf sends a formatted line
func (s *Writer) Printf(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	// Handle \r for progress: just send the line as output
	text = strings.TrimRight(text, "\r\n")
	if text == "" {
		return
	}
	s.writeLine(text)
}

func (s *Writer) writeLine(text string) {
	s.mu.Lock()
	s.lineNum++
	num := s.lineNum
	s.mu.Unlock()

	s.sendEvent("output", map[string]interface{}{
		"line":     text,
		"line_num": num,
	})

	// Save to DB (best effort)
	if s.db != nil {
		models.InsertRunOutput(s.db, s.runID, num, text)
	}
}

func (s *Writer) persistEvent(eventType string, data interface{}) {
	if s.db == nil {
		return
	}
	s.mu.Lock()
	s.eventSeq++
	seq := s.eventSeq
	s.mu.Unlock()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	models.InsertRunEvent(s.db, s.runID, seq, eventType, string(jsonData))
}

func (s *Writer) SendTable(headers []string, rows [][]string) {
	data := map[string]interface{}{
		"headers": headers,
		"rows":    rows,
	}
	s.sendEvent("table", data)
	s.persistEvent("table", data)
}

func (s *Writer) SendGroupedTable(title, group string, headers []string, rows [][]string) {
	data := map[string]interface{}{
		"headers": headers,
		"rows":    rows,
	}
	if title != "" {
		data["title"] = title
	}
	if group != "" {
		data["group"] = group
	}
	s.sendEvent("table", data)
	s.persistEvent("table", data)
}

func (s *Writer) SendGantt(data interface{}) {
	s.sendEvent("gantt", data)
	s.persistEvent("gantt", data)
}

func (s *Writer) SendFile(filename, content string) {
	data := map[string]string{
		"filename": filename,
		"content":  content,
	}
	s.sendEvent("file", data)
	s.persistEvent("file", data)
}

func (s *Writer) RunID() int {
	return s.runID
}

func (s *Writer) DB() *sql.DB {
	return s.db
}
