package store

import (
	"encoding/json"
	"testing"
)

func TestHarnessPayloadNormalizesEmptyValue(t *testing.T) {
	t.Parallel()

	if got := string(harnessPayload(nil)); got != "null" {
		t.Fatalf("harnessPayload(nil) = %q, want null", got)
	}
	payload := json.RawMessage(`{"ok":true}`)
	if got := string(harnessPayload(payload)); got != string(payload) {
		t.Fatalf("harnessPayload(payload) = %q, want %q", got, payload)
	}
}
