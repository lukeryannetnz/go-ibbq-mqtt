package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleShutdownRequiresPost(t *testing.T) {
	server := &webServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/shutdown", nil)
	rec := httptest.NewRecorder()

	server.handleShutdown(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleShutdownPassesPasswordToShutdownFunc(t *testing.T) {
	var gotPassword string
	server := &webServer{
		shutdownFunc: func(password string) error {
			gotPassword = password
			return nil
		},
	}

	body, err := json.Marshal(map[string]string{"password": "secret"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleShutdown(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if gotPassword != "secret" {
		t.Fatalf("password = %q, want %q", gotPassword, "secret")
	}
}

func TestHandleShutdownRejectsInvalidJSON(t *testing.T) {
	server := &webServer{}
	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()

	server.handleShutdown(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
