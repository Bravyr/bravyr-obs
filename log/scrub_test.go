package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestScrubWriter_redactsPassword(t *testing.T) {
	var buf bytes.Buffer
	sw := newScrubWriter(&buf, DefaultDenylist)

	input := `{"level":"info","password":"secret123","message":"test"}` + "\n"
	if _, err := sw.Write([]byte(input)); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	var event map[string]string
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("json.Unmarshal error: %v (output: %q)", err, buf.String())
	}

	if event["password"] != "[REDACTED]" {
		t.Fatalf("expected password to be [REDACTED], got %q", event["password"])
	}
	if event["message"] != "test" {
		t.Fatalf("expected message to be preserved, got %q", event["message"])
	}
}

func TestScrubWriter_allowsSafeFields(t *testing.T) {
	var buf bytes.Buffer
	sw := newScrubWriter(&buf, DefaultDenylist)

	input := `{"level":"info","username":"john","service":"api"}` + "\n"
	if _, err := sw.Write([]byte(input)); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "REDACTED") {
		t.Fatalf("safe fields should not be redacted, got: %s", output)
	}
}

func TestScrubWriter_caseInsensitive(t *testing.T) {
	var buf bytes.Buffer
	sw := newScrubWriter(&buf, DefaultDenylist)

	input := `{"Password":"hunter2"}` + "\n"
	if _, err := sw.Write([]byte(input)); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	var event map[string]string
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if event["Password"] != "[REDACTED]" {
		t.Fatalf("expected Password to be [REDACTED], got %q", event["Password"])
	}
}

func TestScrubWriter_substringMatch(t *testing.T) {
	var buf bytes.Buffer
	sw := newScrubWriter(&buf, DefaultDenylist)

	input := `{"db_password":"pg123","auth_token":"abc"}` + "\n"
	if _, err := sw.Write([]byte(input)); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	var event map[string]string
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if event["db_password"] != "[REDACTED]" {
		t.Fatalf("expected db_password to be [REDACTED], got %q", event["db_password"])
	}
	if event["auth_token"] != "[REDACTED]" {
		t.Fatalf("expected auth_token to be [REDACTED], got %q", event["auth_token"])
	}
}

func TestScrubWriter_nonJSON(t *testing.T) {
	var buf bytes.Buffer
	sw := newScrubWriter(&buf, DefaultDenylist)

	input := "plain text line\n"
	n, err := sw.Write([]byte(input))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("expected n=%d, got %d", len(input), n)
	}
	if buf.String() != input {
		t.Fatalf("expected passthrough, got %q", buf.String())
	}
}

func TestDefaultDenylist_containsExpected(t *testing.T) {
	expected := []string{"password", "token", "email", "secret", "api_key"}
	for _, e := range expected {
		found := false
		for _, d := range DefaultDenylist {
			if d == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in DefaultDenylist", e)
		}
	}
}
