package log

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	seqBufCap       = 1024
	seqBatchSize    = 100
	seqFlushTimeout = 500 * time.Millisecond
	seqHTTPTimeout  = 10 * time.Second
)

// seqWriter is an async, batching zerolog writer that ships CLEF-formatted
// JSON events to a Seq server via HTTP POST. It is safe for concurrent use.
//
// CLEF (Compact Log Event Format) is Seq's native ingestion format. The only
// difference from zerolog's output is the field names for the three reserved
// fields: @t (time), @l (level), @mt (message template).
type seqWriter struct {
	url    string
	apiKey string
	client *http.Client
	buf    chan []byte
	wg     sync.WaitGroup

	closeOnce sync.Once

	// dropped counts events silently discarded when the channel was full.
	dropped atomic.Int64

	// sendFailed counts events lost due to delivery failures.
	sendFailed atomic.Int64
}

// newSeqWriter creates a seqWriter and starts its background flush goroutine.
// The caller must call Close() to drain and shut down.
func newSeqWriter(url, apiKey string) *seqWriter {
	w := &seqWriter{
		url:    url,
		apiKey: apiKey,
		buf:    make(chan []byte, seqBufCap),
		client: &http.Client{
			Timeout: seqHTTPTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
	}

	w.wg.Add(1)
	go w.run()

	return w
}

// Write implements io.Writer. It rewrites the three reserved zerolog field
// names to their CLEF equivalents, copies the payload (zerolog reuses the
// underlying buffer), and enqueues it for async delivery. If the channel is
// full the event is dropped and the dropped counter is incremented.
func (w *seqWriter) Write(p []byte) (int, error) {
	// Rewrite field names: "time" → "@t", "level" → "@l", "message" → "@mt".
	// bytes.Replace with n=1 targets only the first occurrence. This is safe
	// because zerolog emits its built-in fields first (level, then time via
	// Timestamp), user fields in the middle, and message last (written by Msg).
	// The n=1 replacement therefore always hits the zerolog-emitted key.
	out := bytes.Replace(p, []byte(`"time"`), []byte(`"@t"`), 1)
	out = bytes.Replace(out, []byte(`"level"`), []byte(`"@l"`), 1)
	out = bytes.Replace(out, []byte(`"message"`), []byte(`"@mt"`), 1)

	// Copy because zerolog reuses the buffer backing p after Write returns.
	copied := make([]byte, len(out))
	copy(copied, out)

	select {
	case w.buf <- copied:
	default:
		w.dropped.Add(1)
	}

	return len(p), nil
}

// Close signals the background goroutine to drain and stop. It blocks until
// all buffered events have been flushed. Safe to call multiple times.
func (w *seqWriter) Close() error {
	w.closeOnce.Do(func() {
		close(w.buf)
	})
	w.wg.Wait()
	return nil
}

// run is the background goroutine that batches and ships events to Seq.
func (w *seqWriter) run() {
	defer w.wg.Done()

	batch := make([][]byte, 0, seqBatchSize)
	ticker := time.NewTicker(seqFlushTimeout)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Single-attempt POST; failed batches are retried on the next tick.
		if err := w.send(batch); err != nil {
			w.sendFailed.Add(int64(len(batch)))
			fmt.Fprintf(os.Stderr, "seq: batch delivery failed (%d events): %v\n", len(batch), err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-w.buf:
			if !ok {
				// Channel closed — drain remaining events and exit.
				flush()
				return
			}
			batch = append(batch, event)
			if len(batch) >= seqBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// send POSTs a CLEF batch to Seq. Returns nil on success. Failed batches are
// retried implicitly on the next ticker flush cycle.
func (w *seqWriter) send(batch [][]byte) error {
	body := bytes.Join(batch, []byte("\n"))
	return w.post(body)
}

// post performs a single HTTP POST to the Seq ingestion endpoint.
func (w *seqWriter) post(body []byte) error {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		w.url+"/api/events/raw",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("seq: building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/vnd.serilog.clef")
	if w.apiKey != "" {
		req.Header.Set("X-Seq-ApiKey", w.apiKey)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("seq: http request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("seq: server returned %d", resp.StatusCode)
	}

	return nil
}
