package core

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

// ServeBridge is the protocol-neutral browser ↔ agent WebSocket host.
func (s *Server) ServeBridge(spec sessioncore.BridgeSpec, w http.ResponseWriter, r *http.Request) {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		http.Error(w, "ticket required", http.StatusBadRequest)
		return
	}
	base, raw, err := s.verifySessionTicket(ticket)
	if err != nil {
		http.Error(w, "invalid ticket: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if !contains(spec.ExpectedTypes, base.Type) {
		http.Error(w, "ticket type mismatch: "+base.Type, http.StatusBadRequest)
		return
	}
	prepared := sessioncore.BridgePrepared{}
	if spec.Prepare != nil {
		prepared, err = spec.Prepare(base, raw)
		if err != nil {
			http.Error(w, "invalid protocol claims: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	protocol := spec.Protocol
	if protocol == "" {
		protocol = base.Type
	}
	browserWS, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer browserWS.Close()

	fields := auditFieldsFromBase(base)
	type waitResult struct {
		ws  *websocket.Conn
		err error
	}
	s.pending.Arm(base.SessionID)
	waitDone := make(chan waitResult, 1)
	go func() {
		ws, err := s.pending.Wait(base.SessionID, 20*time.Second)
		waitDone <- waitResult{ws, err}
	}()
	if err := s.openAgentSession(base.AssetID, base.SessionID, ticket, protocol, prepared.Params); err != nil {
		_ = browserWS.WriteMessage(websocket.TextMessage, []byte("ERROR: "+err.Error()))
		s.auditEnd(spec.OperationType, fields, false, "",
			mergeDetail(prepared.StartDetail, map[string]any{"error": err.Error()}))
		return
	}
	wr := <-waitDone
	if wr.err != nil {
		_ = browserWS.WriteMessage(websocket.TextMessage, []byte("ERROR: "+wr.err.Error()))
		s.auditEnd(spec.OperationType, fields, false, "",
			mergeDetail(prepared.StartDetail, map[string]any{"error": wr.err.Error()}))
		return
	}
	agentWS := wr.ws
	defer agentWS.Close()
	log.Printf("bridge %s session=%s asset=%s", protocol, base.SessionID, base.AssetID)
	s.auditStart(spec.OperationType, fields, prepared.StartDetail)
	if prepared.Interrupt != nil {
		s.attachLiveOpFinalize(base.SessionID, prepared.Interrupt)
	}

	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(browserWS, keepDone)
	sessioncore.StartKeepAlive(agentWS, keepDone)
	var bytesIn, bytesOut int64
	errCh := make(chan error, 2)
	go func() {
		errCh <- sessioncore.CopyWSTee(
			agentWS, browserWS, &bytesIn, prepared.BrowserToAgent, prepared.BrowserToAgentSize)
	}()
	go func() {
		errCh <- sessioncore.CopyWSTee(
			browserWS, agentWS, &bytesOut, prepared.AgentToBrowser, prepared.AgentToBrowserSize)
	}()
	<-errCh
	_ = browserWS.Close()
	_ = agentWS.Close()
	if prepared.Flush != nil {
		prepared.Flush()
	}
	endDetail := map[string]any{
		"bytesIn":  atomic.LoadInt64(&bytesIn),
		"bytesOut": atomic.LoadInt64(&bytesOut),
	}
	success := true
	if prepared.Finish != nil {
		ok, detail := prepared.Finish()
		success = ok
		endDetail = mergeDetail(endDetail, detail)
	}
	s.auditEnd(spec.OperationType, fields, success, "", endDetail)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// OpenAgentSession sends protocol-owned params without inspecting them.
func (s *Server) OpenAgentSession(assetID, sessionID, ticket, protocol string, params any) error {
	return s.openAgentSession(assetID, sessionID, ticket, protocol, params)
}

func (s *Server) openAgentSession(assetID, sessionID, ticket, protocol string, params any) error {
	agent, ok := s.agents.Get(assetID)
	if !ok {
		return fmt.Errorf("agent offline")
	}
	payload, err := control.BuildOpenSession(sessionID, protocol, ticket, params)
	if err != nil {
		return err
	}
	msg, err := control.Marshal("open_session", sessionID, payload)
	if err != nil {
		return err
	}
	return agent.Send(msg)
}
