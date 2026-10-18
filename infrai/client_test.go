package infrai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishRequestBoundary(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/realtime/publish" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header was not set")
		}
		if r.Header.Get("Idempotency-Key") != "publish-asset-42-r2" {
			t.Errorf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["channel"] != "creator.7.asset.42" || body["event"] != "asset.ready" || body["account_id"] != "creator-7" {
			t.Errorf("unexpected body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client, err := NewClient("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	err = client.Publish(context.Background(), "creator.7.asset.42", "asset.ready", map[string]string{"asset_id": "asset-42"}, "creator-7", "publish-asset-42-r2")
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestEnvelopeErrorIsDecodedBeforeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"message":"channel is required"},"metadata":{}}`))
	}))
	defer server.Close()

	client, err := NewClient("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateChannel(context.Background(), "", "private", "", "room-empty")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Message != "channel is required" || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("APIError = %#v", apiErr)
	}
}

func TestDeleteChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/realtime/channel/delete/creator.7.asset.42" {
			t.Errorf("unexpected delete request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{},"error":null,"metadata":{}}`))
	}))
	defer server.Close()
	client, err := NewClient("test-key", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteChannel(context.Background(), "creator.7.asset.42"); err != nil {
		t.Fatal(err)
	}
}
