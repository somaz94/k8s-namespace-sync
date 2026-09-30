package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// logLine parses args onto zapOptions, logs one line and returns it.
func logLine(t *testing.T, args ...string) []byte {
	t.Helper()
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	opts := zapOptions()
	opts.BindFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	var buf bytes.Buffer
	zap.New(zap.UseFlagOptions(&opts), zap.WriteTo(&buf)).Info("hello")
	return buf.Bytes()
}

func TestZapOptionsHonourEncoderFlag(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		wantJSON bool
	}{
		{"default", nil, false},
		{"json", []string{"--zap-encoder=json"}, true},
		{"console", []string{"--zap-encoder=console"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := logLine(t, tc.args...)

			var entry map[string]any
			isJSON := json.Unmarshal(out, &entry) == nil
			if isJSON != tc.wantJSON {
				t.Fatalf("JSON output = %v, want %v: %q", isJSON, tc.wantJSON, out)
			}
			if tc.wantJSON && (entry["msg"] != "hello" || entry["level"] != "info") {
				t.Errorf("expected the msg and level field names to survive, got %v", entry)
			}
		})
	}
}

func TestZapOptionsHonourTimeEncodingFlag(t *testing.T) {
	var entry map[string]any
	if err := json.Unmarshal(logLine(t, "--zap-encoder=json", "--zap-time-encoding=epoch"), &entry); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if _, ok := entry["ts"].(float64); !ok {
		t.Errorf("expected an epoch timestamp, got %v", entry["ts"])
	}

	if err := json.Unmarshal(logLine(t, "--zap-encoder=json"), &entry); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if _, ok := entry["ts"].(string); !ok {
		t.Errorf("expected the ISO8601 default, got %v", entry["ts"])
	}
}
