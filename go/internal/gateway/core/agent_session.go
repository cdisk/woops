package core

import "net/http"

func (s *Server) handleAgentSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	ticket := r.URL.Query().Get("ticket")
	assetID := r.Header.Get("X-Asset-Id")
	token := r.Header.Get("X-Agent-Token")
	if sessionID == "" || ticket == "" || assetID == "" || token == "" {
		http.Error(w, "missing session params", http.StatusBadRequest)
		return
	}
	if err := s.postJSON("/api/internal/agent/auth", map[string]any{
		"assetId": assetID, "agentToken": token,
	}, &map[string]any{}); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	claims, _, err := s.verifySessionTicket(ticket)
	if err != nil {
		http.Error(w, "invalid session ticket", http.StatusUnauthorized)
		return
	}
	s.extensionsMu.RLock()
	_, allowed := s.allowedTicketTypes[claims.Type]
	s.extensionsMu.RUnlock()
	if claims.SessionID != sessionID || claims.AssetID != assetID || !allowed {
		http.Error(w, "session ticket mismatch", http.StatusForbidden)
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if !s.pending.Deliver(sessionID, ws) {
		_ = ws.Close()
		return
	}
}
