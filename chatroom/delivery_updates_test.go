package chatroom

import "testing"

func TestDecideDeliveryEvent(t *testing.T) {
	tests := []struct {
		name      string
		update    ProcessingUpdate
		wantEvent string
		wantError bool
	}{
		{
			name:      "processing stays non-deliverable",
			update:    ProcessingUpdate{CreatorID: "creator-7", AssetID: "asset-42", Revision: 1, State: "processing"},
			wantEvent: "asset.processing",
		},
		{
			name:      "ready publishes delivery location",
			update:    ProcessingUpdate{CreatorID: "creator-7", AssetID: "asset-42", Revision: 2, State: "ready", Download: "https://downloads.example/asset-42"},
			wantEvent: "asset.ready",
		},
		{
			name:      "ready requires delivery location",
			update:    ProcessingUpdate{CreatorID: "creator-7", AssetID: "asset-42", Revision: 2, State: "ready"},
			wantError: true,
		},
		{
			name:      "review state is explicit",
			update:    ProcessingUpdate{CreatorID: "creator-7", AssetID: "asset-42", Revision: 3, State: "review_required"},
			wantEvent: "asset.review_required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, payload, err := DecideDeliveryEvent(test.update)
			if test.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecideDeliveryEvent() error = %v", err)
			}
			if event != test.wantEvent {
				t.Fatalf("event = %q, want %q", event, test.wantEvent)
			}
			if payload.AssetID != test.update.AssetID || payload.Revision != test.update.Revision {
				t.Fatalf("payload = %#v", payload)
			}
			if test.update.State != "ready" && payload.Download != "" {
				t.Fatalf("non-ready payload exposed download %q", payload.Download)
			}
		})
	}
}
