package chatroom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"example.com/creator-chat-rooms/infrai"
)

type RealtimeAPI interface {
	CreateChannel(context.Context, string, string, string, string) (infrai.Channel, error)
	DeleteChannel(context.Context, string) error
	IssueToken(context.Context, string, []string, []string, int, string) (infrai.RealtimeToken, error)
	Publish(context.Context, string, string, any, string, string) error
	GetPresence(context.Context, string) (infrai.Presence, error)
}

type Service struct {
	realtime RealtimeAPI
}

func NewService(realtime RealtimeAPI) *Service {
	return &Service{realtime: realtime}
}

type OpenRoomInput struct {
	CreatorID    string `json:"creator_id"`
	SubscriberID string `json:"subscriber_id"`
	AssetID      string `json:"asset_id"`
}

type OpenRoomResult struct {
	Channel string `json:"channel"`
	Token   string `json:"token"`
}

func (s *Service) OpenRoom(ctx context.Context, input OpenRoomInput) (OpenRoomResult, error) {
	if input.CreatorID == "" || input.SubscriberID == "" || input.AssetID == "" {
		return OpenRoomResult{}, errors.New("creator_id, subscriber_id, and asset_id are required")
	}
	channel := fmt.Sprintf("creator.%s.asset.%s", input.CreatorID, input.AssetID)
	key := stableKey("room", channel)
	if _, err := s.realtime.CreateChannel(ctx, channel, "private", "", key); err != nil {
		return OpenRoomResult{}, err
	}
	token, err := s.realtime.IssueToken(ctx, input.SubscriberID, []string{channel}, []string{"subscribe"}, 900, stableKey("token", channel, input.SubscriberID))
	if err != nil {
		return OpenRoomResult{}, err
	}
	return OpenRoomResult{Channel: channel, Token: token.Token}, nil
}

type ProcessingUpdate struct {
	CreatorID string `json:"creator_id"`
	AssetID   string `json:"asset_id"`
	Revision  int    `json:"revision"`
	State     string `json:"state"`
	Download  string `json:"download,omitempty"`
}

type DeliveryEvent struct {
	AssetID  string `json:"asset_id"`
	Revision int    `json:"revision"`
	State    string `json:"state"`
	Download string `json:"download,omitempty"`
}

func DecideDeliveryEvent(update ProcessingUpdate) (string, DeliveryEvent, error) {
	if update.CreatorID == "" || update.AssetID == "" || update.Revision < 1 {
		return "", DeliveryEvent{}, errors.New("creator_id, asset_id, and a positive revision are required")
	}
	event := DeliveryEvent{AssetID: update.AssetID, Revision: update.Revision, State: update.State}
	switch update.State {
	case "processing":
		return "asset.processing", event, nil
	case "ready":
		if strings.TrimSpace(update.Download) == "" {
			return "", DeliveryEvent{}, errors.New("download is required when state is ready")
		}
		event.Download = update.Download
		return "asset.ready", event, nil
	case "review_required":
		return "asset.review_required", event, nil
	default:
		return "", DeliveryEvent{}, fmt.Errorf("unsupported content state %q", update.State)
	}
}

func (s *Service) PublishUpdate(ctx context.Context, update ProcessingUpdate) (string, error) {
	eventName, event, err := DecideDeliveryEvent(update)
	if err != nil {
		return "", err
	}
	channel := fmt.Sprintf("creator.%s.asset.%s", update.CreatorID, update.AssetID)
	key := stableKey("publish", channel, fmt.Sprint(update.Revision), update.State)
	if err := s.realtime.Publish(ctx, channel, eventName, event, update.CreatorID, key); err != nil {
		return "", err
	}
	return eventName, nil
}

func (s *Service) Presence(ctx context.Context, channel string) (infrai.Presence, error) {
	if strings.TrimSpace(channel) == "" {
		return infrai.Presence{}, errors.New("channel is required")
	}
	return s.realtime.GetPresence(ctx, channel)
}

func (s *Service) CloseRoom(ctx context.Context, channel string) error {
	if strings.TrimSpace(channel) == "" {
		return errors.New("channel is required")
	}
	return s.realtime.DeleteChannel(ctx, channel)
}

func stableKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}
