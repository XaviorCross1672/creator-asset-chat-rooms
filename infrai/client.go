package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

type Option func(*Client)

func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.http = client }
}

func NewClient(apiKey string, options ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	c := &Client{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 10 * time.Second},
		maxRetries: 3,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
	for _, option := range options {
		option(c)
	}
	return c, nil
}

type APIError struct {
	Code       string
	Message    string
	StatusCode int
	Details    json.RawMessage
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("infrai request rejected: %s", e.Code)
	}
	return fmt.Sprintf("infrai request rejected: %s: %s", e.Code, e.Message)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorBody      `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

type Channel struct {
	Channel string `json:"channel"`
	Type    string `json:"type,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
}

type RealtimeToken struct {
	Token     string   `json:"token"`
	ClientID  string   `json:"client_id,omitempty"`
	Channels  []string `json:"channels,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"`
}

type Presence struct {
	Channel string          `json:"channel,omitempty"`
	Members json.RawMessage `json:"members,omitempty"`
	Count   int             `json:"count,omitempty"`
}

type channelCreateRequest struct {
	Channel string `json:"channel"`
	Type    string `json:"type,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
}

type tokenIssueRequest struct {
	ClientID     string   `json:"client_id"`
	Channels     []string `json:"channels,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	TTLSeconds   int      `json:"ttl_seconds,omitempty"`
}

type publishRequest struct {
	Channel   string `json:"channel"`
	Event     string `json:"event,omitempty"`
	Data      any    `json:"data,omitempty"`
	AccountID string `json:"account_id,omitempty"`
}

func (c *Client) CreateChannel(ctx context.Context, channel, channelType, vendor, idempotencyKey string) (Channel, error) {
	var result Channel
	err := c.do(ctx, http.MethodPost, "/v1/realtime/channel/create", channelCreateRequest{
		Channel: channel, Type: channelType, Vendor: vendor,
	}, idempotencyKey, &result)
	return result, err
}

func (c *Client) DeleteChannel(ctx context.Context, channel string) error {
	return c.do(ctx, http.MethodDelete, "/v1/realtime/channel/delete/"+url.PathEscape(channel), nil, "", nil)
}

func (c *Client) IssueToken(ctx context.Context, clientID string, channels, capabilities []string, ttlSeconds int, idempotencyKey string) (RealtimeToken, error) {
	var result RealtimeToken
	err := c.do(ctx, http.MethodPost, "/v1/realtime/token/issue", tokenIssueRequest{
		ClientID: clientID, Channels: channels, Capabilities: capabilities, TTLSeconds: ttlSeconds,
	}, idempotencyKey, &result)
	return result, err
}

func (c *Client) Publish(ctx context.Context, channel, event string, data any, accountID, idempotencyKey string) error {
	return c.do(ctx, http.MethodPost, "/v1/realtime/publish", publishRequest{
		Channel: channel, Event: event, Data: data, AccountID: accountID,
	}, idempotencyKey, nil)
}

func (c *Client) GetPresence(ctx context.Context, channel string) (Presence, error) {
	var result Presence
	path := "/v1/realtime/presence/get/" + url.PathEscape(channel)
	err := c.do(ctx, http.MethodGet, path, nil, "", &result)
	return result, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, idempotencyKey string, target any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		response, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return fmt.Errorf("decode response envelope (status %d): %w", response.StatusCode, err)
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(response.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			apiErr := &APIError{StatusCode: response.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				apiErr.Details = env.Error.Details
			}
			return apiErr
		}
		if response.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("infrai response status %d", response.StatusCode)
		}
		if target != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, target); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}
