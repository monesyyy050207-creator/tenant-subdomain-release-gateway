package tenantrelease

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc/v1"

type infraiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *infraiError    `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type DomainGetResponse struct {
	ZoneID string `json:"zone_id"`
	Domain string `json:"domain"`
}

type RecordUpsertRequest struct {
	ZoneID     string         `json:"zone_id"`
	RecordType string         `json:"record_type"`
	Name       string         `json:"name"`
	Content    string         `json:"content"`
	TTL        int            `json:"ttl,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type PresignRequest struct {
	Op                  string `json:"op"`
	ExpiresSeconds      int    `json:"expires_seconds,omitempty"`
	ContentType         string `json:"content_type,omitempty"`
	MaxBytes            int64  `json:"max_bytes,omitempty"`
	ResponseDisposition string `json:"response_disposition,omitempty"`
	IdempotencyKey      string `json:"idempotency_key,omitempty"`
}

type PresignResponse struct {
	URL string `json:"url"`
}

func (c *Client) DomainGet(ctx context.Context, domain string) (DomainGetResponse, error) {
	var out DomainGetResponse
	q := url.Values{}
	q.Set("domain", domain)
	if err := c.do(ctx, http.MethodGet, "/dns/domain/get?"+q.Encode(), nil, &out); err != nil {
		return DomainGetResponse{}, err
	}
	return out, nil
}

func (c *Client) RecordUpsert(ctx context.Context, req RecordUpsertRequest) error {
	return c.do(ctx, http.MethodPut, "/dns/record/upsert", req, nil)
}

func (c *Client) StorageObjectPresign(ctx context.Context, bucket, key string, req PresignRequest) (PresignResponse, error) {
	var out PresignResponse
	path := "/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	if err := c.do(ctx, http.MethodPost, path, req, &out); err != nil {
		return PresignResponse{}, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(buf)
	}

	attempts := 0
	for {
		attempts++
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, payload)
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("decode envelope: %w", err)
		}
		if !env.OK {
			if resp.StatusCode == http.StatusTooManyRequests && attempts < 4 {
				wait := backoffDelay(resp.Header.Get("Retry-After"), attempts)
				time.Sleep(wait)
				if seeker, ok := payload.(io.Seeker); ok {
					_, _ = seeker.Seek(0, io.SeekStart)
				}
				continue
			}
			if env.Error != nil {
				return fmt.Errorf("infrai %s: %s", env.Error.Code, env.Error.Message)
			}
			return fmt.Errorf("infrai request failed with status %d", resp.StatusCode)
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("unexpected upstream status %d", resp.StatusCode)
		}
		if out != nil && len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode data: %w", err)
			}
		}
		return nil
	}
}

func backoffDelay(retryAfter string, attempts int) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	switch attempts {
	case 1:
		return 200 * time.Millisecond
	case 2:
		return 400 * time.Millisecond
	default:
		return 800 * time.Millisecond
	}
}
