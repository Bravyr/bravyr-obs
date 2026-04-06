package log

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSeqWriter_CLEFRewrite(t *testing.T) {
	w := &seqWriter{
		buf: make(chan []byte, 16),
	}

	input := []byte(`{"level":"info","time":"2026-04-06T00:00:00Z","message":"hello"}`)
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write() returned n=%d, want %d", n, len(input))
	}

	got := <-w.buf

	var event map[string]any
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatalf("json.Unmarshal failed: %v (output: %q)", err, got)
	}

	if _, ok := event["@t"]; !ok {
		t.Errorf("expected @t field, got keys: %v", event)
	}
	if _, ok := event["@l"]; !ok {
		t.Errorf("expected @l field, got keys: %v", event)
	}
	if _, ok := event["@mt"]; !ok {
		t.Errorf("expected @mt field, got keys: %v", event)
	}
	if _, ok := event["level"]; ok {
		t.Error("original 'level' key must be removed")
	}
	if _, ok := event["time"]; ok {
		t.Error("original 'time' key must be removed")
	}
	if _, ok := event["message"]; ok {
		t.Error("original 'message' key must be removed")
	}
}

func TestSeqWriter_dropOnFull(t *testing.T) {
	w := &seqWriter{
		buf: make(chan []byte, 2),
	}

	for i := range 2 {
		if _, err := w.Write([]byte(`{"level":"info","time":"t","message":"` + string(rune('a'+i)) + `"}`)); err != nil {
			t.Fatalf("Write() error: %v", err)
		}
	}

	if _, err := w.Write([]byte(`{"level":"info","time":"t","message":"overflow"}`)); err != nil {
		t.Fatalf("Write() error on drop: %v", err)
	}

	if got := w.dropped.Load(); got != 1 {
		t.Fatalf("expected dropped=1, got %d", got)
	}
}

func TestSeqWriter_batchFlush(t *testing.T) {
	var mu sync.Mutex
	var received [][]byte
	var reqCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		if ct := r.Header.Get("Content-Type"); ct != "application/vnd.serilog.clef" {
			t.Errorf("expected CLEF content-type, got %q", ct)
		}
		if key := r.Header.Get("X-Seq-ApiKey"); key != "test-key" {
			t.Errorf("expected X-Seq-ApiKey=test-key, got %q", key)
		}

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		mu.Lock()
		received = append(received, buf.Bytes())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sw := newSeqWriter(srv.URL, "test-key")

	const n = 5
	for i := range n {
		line := []byte(`{"level":"info","time":"t","message":"` + string(rune('a'+i)) + `"}`)
		if _, err := sw.Write(line); err != nil {
			t.Fatalf("Write() error: %v", err)
		}
	}

	if err := sw.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if reqCount.Load() == 0 {
		t.Fatal("expected at least one POST to Seq server")
	}

	mu.Lock()
	defer mu.Unlock()
	total := 0
	for _, body := range received {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		for scanner.Scan() {
			if line := scanner.Bytes(); len(line) > 0 {
				total++
			}
		}
	}
	if total != n {
		t.Fatalf("expected %d events delivered, got %d", n, total)
	}
}

func TestSeqWriter_sendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sw := &seqWriter{
		url:    srv.URL,
		apiKey: "",
		buf:    make(chan []byte, 16),
		client: srv.Client(),
	}

	batch := [][]byte{[]byte(`{"@t":"t","@l":"info","@mt":"error test"}`)}
	err := sw.send(batch)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestSeqWriter_dropOn4xx(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	sw := &seqWriter{
		url:    srv.URL,
		apiKey: "",
		buf:    make(chan []byte, 16),
		client: srv.Client(),
	}

	batch := [][]byte{[]byte(`{"@t":"t","@l":"info","@mt":"drop test"}`)}
	err := sw.send(batch)
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 HTTP call (no retry on 4xx), got %d", got)
	}
}

func TestSeqWriter_close(t *testing.T) {
	var delivered atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scanner := bufio.NewScanner(r.Body)
		for scanner.Scan() {
			if len(scanner.Bytes()) > 0 {
				delivered.Add(1)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sw := newSeqWriter(srv.URL, "")

	for range 10 {
		_, _ = sw.Write([]byte(`{"level":"info","time":"t","message":"drain test"}`))
	}

	done := make(chan error, 1)
	go func() { done <- sw.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5s")
	}

	if n := delivered.Load(); n != 10 {
		t.Fatalf("expected 10 events delivered after close, got %d", n)
	}
}

func TestSeqWriter_doubleClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // makes the port refuse connections
	sw := newSeqWriter(srv.URL, "")
	if err := sw.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	// Second close must not panic.
	if err := sw.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}

func TestNew_withSeqURL(t *testing.T) {
	var delivered atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scanner := bufio.NewScanner(r.Body)
		for scanner.Scan() {
			if len(scanner.Bytes()) > 0 {
				delivered.Add(1)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	l, err := New(Config{
		ServiceName: "test-svc",
		Level:       "info",
		SeqURL:      srv.URL,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	l.Info().Msg("integration test")

	if err := l.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}

	if n := delivered.Load(); n != 1 {
		t.Fatalf("expected 1 event delivered via Seq, got %d", n)
	}
}
