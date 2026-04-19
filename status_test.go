package main

import (
	"testing"
	"time"
)

func TestTestStatusJson(t *testing.T) {
	data := &status{"working!!", "2026-04-19T10:11:12Z"}
	expected := "{\"Status\":\"working!!\",\"Timestamp\":\"2026-04-19T10:11:12Z\"}" // {"Status":"working!!","Timestamp":"2026-04-19T10:11:12Z"}
	var got = data.toJson()

	if got != expected {
		t.Errorf("got %s; want %s", got, expected)
	}
}

func TestPublishStatusIncludesTimestamp(t *testing.T) {
	before := time.Now().UTC()
	s := &status{
		Status:    "Connected",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if _, err := time.Parse(time.RFC3339, s.Timestamp); err != nil {
		t.Fatalf("timestamp should be RFC3339: %v", err)
	}

	if s.Timestamp < before.Format(time.RFC3339) {
		t.Fatalf("timestamp %s should not be before %s", s.Timestamp, before.Format(time.RFC3339))
	}
}
