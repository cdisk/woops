package opsctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type TicketResponse struct {
	SessionID   string `json:"sessionId"`
	Protocol    string `json:"protocol"`
	Ticket      string `json:"ticket"`
	ExpiresAt   string `json:"expiresAt"`
	BrowserWS   string `json:"browserWs"`
	Implemented bool   `json:"implemented"`
	TransferID  string `json:"transferId"`
	Direction   string `json:"direction"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Fingerprint string `json:"fingerprint"`
	Abort       bool   `json:"abort"`
}

// HTTPError preserves the status and response body so long-running commands can
// distinguish authentication failures from retryable gateway outages.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("ticket request failed (%d): %s", e.StatusCode, truncate(e.Body, 300))
}

func (t *TicketResponse) AsTransfer() *TransferTicket {
	if t == nil {
		return nil
	}
	return &TransferTicket{
		BrowserWS:   t.BrowserWS,
		TransferID:  t.TransferID,
		Direction:   t.Direction,
		Path:        t.Path,
		Size:        t.Size,
		Fingerprint: t.Fingerprint,
		Abort:       t.Abort,
	}
}

type Client struct {
	cfg Config
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) CreateTicket(action string, meta map[string]any) (*TicketResponse, error) {
	return c.CreateTicketContext(context.Background(), action, meta)
}

func (c *Client) CreateTicketContext(ctx context.Context, action string, meta map[string]any) (*TicketResponse, error) {
	body := map[string]any{
		"action": action,
		"meta":   meta,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	// server = Gateway HTTPS base; /api/opsctl/tickets is proxied to control-api.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Server+"/api/opsctl/tickets", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	var out TicketResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("bad ticket response: %w", err)
	}
	if out.BrowserWS == "" {
		return nil, fmt.Errorf("ticket response missing browserWs")
	}
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
