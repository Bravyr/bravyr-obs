package log

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// DefaultDenylist contains field names that are scrubbed from log output
// when PII scrubbing is enabled. Matching is case-insensitive and uses
// substring containment (e.g., "db_password" matches "password").
var DefaultDenylist = []string{
	"password", "passwd", "secret", "token",
	"authorization", "cookie", "email", "phone", "ssn",
	"credit_card", "card_number", "api_key",
}

// scrubWriter wraps an io.Writer and redacts top-level JSON fields whose
// names contain any entry from the denylist. Non-JSON lines pass through
// unchanged. Note: nested JSON objects are not traversed — only top-level
// field names are checked. Structured loggers like zerolog typically flatten
// fields, so this covers the common case.
type scrubWriter struct {
	w        io.Writer
	denylist []string
}

func newScrubWriter(w io.Writer, denylist []string) *scrubWriter {
	lower := make([]string, len(denylist))
	for i, d := range denylist {
		lower[i] = strings.ToLower(d)
	}
	return &scrubWriter{w: w, denylist: lower}
}

func (s *scrubWriter) Write(p []byte) (int, error) {
	trimmed := bytes.TrimSpace(p)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return s.w.Write(p)
	}

	var event map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return s.w.Write(p)
	}

	changed := false
	for key := range event {
		if s.isDenied(key) {
			event[key] = json.RawMessage(`"[REDACTED]"`)
			changed = true
		}
	}

	if !changed {
		return s.w.Write(p)
	}

	out, err := json.Marshal(event)
	if err != nil {
		return s.w.Write(p)
	}
	out = append(out, '\n')
	n, writeErr := s.w.Write(out)
	if writeErr != nil {
		return n, writeErr
	}
	if n != len(out) {
		return 0, io.ErrShortWrite
	}
	return len(p), nil
}

func (s *scrubWriter) isDenied(key string) bool {
	lower := strings.ToLower(key)
	for _, d := range s.denylist {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}
