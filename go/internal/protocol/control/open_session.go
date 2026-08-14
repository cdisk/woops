package control

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildOpenSession constructs an open_session payload with nested Params only.
func BuildOpenSession(sessionID, protocol, ticket string, params any) (OpenSessionPayload, error) {
	out := OpenSessionPayload{
		SessionID: sessionID,
		Protocol:  strings.ToLower(strings.TrimSpace(protocol)),
		Ticket:    ticket,
	}
	if params == nil {
		return out, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return out, err
	}
	out.Params = raw
	return out, nil
}

// NormalizeOpenSession lowercases protocol; Params must already be nested.
func NormalizeOpenSession(p OpenSessionPayload) OpenSessionPayload {
	p.Protocol = strings.ToLower(strings.TrimSpace(p.Protocol))
	return p
}

// UnmarshalOpenParams decodes nested Params into T.
func UnmarshalOpenParams[T any](p OpenSessionPayload) (T, error) {
	var out T
	p = NormalizeOpenSession(p)
	if len(p.Params) == 0 || string(p.Params) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(p.Params, &out); err != nil {
		return out, fmt.Errorf("open_session params: %w", err)
	}
	return out, nil
}
