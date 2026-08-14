package core

import (
	"encoding/json"
	"fmt"

	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

// verifySessionTicket is the control-plane boundary. Protocol modules decode
// their own fields from raw; gateway core only validates common identity.
func (s *Server) verifySessionTicket(ticket string) (sessioncore.BaseClaims, json.RawMessage, error) {
	var raw json.RawMessage
	if err := s.postJSON("/api/sessions/internal/verify-ticket", map[string]string{"ticket": ticket}, &raw); err != nil {
		return sessioncore.BaseClaims{}, nil, err
	}
	var c sessioncore.BaseClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return sessioncore.BaseClaims{}, nil, err
	}
	if c.SessionID == "" || c.Type == "" || c.AssetID == "" {
		return sessioncore.BaseClaims{}, nil, fmt.Errorf("incomplete ticket claims")
	}
	return c, raw, nil
}
