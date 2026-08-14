package core

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"time"

	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

type auditFields = sessioncore.AuditFields

func auditFieldsFromBase(c sessioncore.BaseClaims) auditFields {
	return auditFields{
		OperationID: c.SessionID,
		AssetID:     c.AssetID,
		UserID:      c.UserID,
		Username:    c.Username,
	}
}

// AuditEvent records a protocol module action using only common ticket claims.
func (s *Server) AuditEvent(operationType string, base sessioncore.BaseClaims, event string, detail map[string]any) {
	s.auditEvent(auditstore.PhaseAction, operationType, auditFieldsFromBase(base), detail,
		map[string]any{"eventType": event})
}

// AuditStart starts an operation owned by an application module.
func (s *Server) AuditStart(operationType string, base sessioncore.BaseClaims, detail map[string]any) {
	s.auditStart(operationType, auditFieldsFromBase(base), detail)
}

// AuditEnd ends an operation owned by an application module.
func (s *Server) AuditEnd(operationType string, base sessioncore.BaseClaims, success bool, detail map[string]any) {
	s.auditEnd(operationType, auditFieldsFromBase(base), success, "", detail)
}

// AttachAuditFinalize installs a shutdown finalizer for an active operation.
func (s *Server) AttachAuditFinalize(operationID string, finalize func() map[string]any) {
	s.attachLiveOpFinalize(operationID, finalize)
}

// auditEvent spools one envelope. A disabled spool, an unknown operation type
// or non-UUID ids make it a no-op: auditing must never break a live session.
// extra carries phase-specific top-level fields (status, success, endedAt).
func (s *Server) auditEvent(
	phase, operationType string, f auditFields, detail, extra map[string]any) {
	if s.audit == nil || operationType == "" {
		return
	}
	if !isUUID(f.OperationID) || !isUUID(f.AssetID) {
		log.Printf("ops-audit skip %s %s: operationId=%q assetId=%q not uuid",
			phase, operationType, f.OperationID, f.AssetID)
		return
	}
	env := map[string]any{
		"phase":         phase,
		"operationId":   f.OperationID,
		"operationType": operationType,
		"assetId":       f.AssetID,
		"occurredAt":    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if isUUID(f.UserID) {
		env["userId"] = f.UserID
	}
	if f.Username != "" {
		env["username"] = f.Username
	}
	if phase == auditstore.PhaseAction || phase == auditstore.PhaseError {
		env["eventId"] = newUUIDv4()
	}
	for k, v := range extra {
		env[k] = v
	}
	if len(detail) > 0 {
		env["detail"] = detail
	}
	if err := s.audit.Append(env); err != nil {
		log.Printf("ops-audit append failed %s %s op=%s: %v", phase, operationType, f.OperationID, err)
	}
}

func (s *Server) auditStart(operationType string, f auditFields, detail map[string]any) {
	if f.OperationID != "" {
		s.liveOps.Store(f.OperationID, liveOp{opType: operationType, fields: f})
	}
	s.auditEvent(auditstore.PhaseStart, operationType, f, detail,
		map[string]any{"status": auditstore.StatusRunning})
}

// attachLiveOpFinalize registers a one-shot END detail builder for an open
// operation so feature-owned resources can be finalized before interruption.
func (s *Server) attachLiveOpFinalize(operationID string, finalize func() map[string]any) {
	if operationID == "" || finalize == nil {
		return
	}
	if v, ok := s.liveOps.Load(operationID); ok {
		op := v.(liveOp)
		op.finalize = finalize
		s.liveOps.Store(operationID, op)
	}
}

// auditEnd closes an operation. Pass status only when it is neither COMPLETED
// nor FAILED (e.g. INTERRUPTED); otherwise it follows success.
func (s *Server) auditEnd(
	operationType string, f auditFields, success bool, status string, detail map[string]any) {
	if f.OperationID != "" {
		s.liveOps.Delete(f.OperationID)
	}
	if status == "" {
		status = auditstore.StatusCompleted
		if !success {
			status = auditstore.StatusFailed
		}
	}
	s.auditEvent(auditstore.PhaseEnd, operationType, f, detail, map[string]any{
		"status":  status,
		"success": success,
		"endedAt": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// interruptLiveOps writes INTERRUPTED END for every operation still open
// (Gateway restart / SIGTERM). Flushes any registered recording finalize so
// replay metadata is preserved. Safe to call with auditing disabled.
func (s *Server) interruptLiveOps(reason string) {
	s.liveOps.Range(func(key, value any) bool {
		op, ok := value.(liveOp)
		if !ok {
			return true
		}
		detail := map[string]any{"reason": reason}
		if op.finalize != nil {
			detail = mergeDetail(detail, op.finalize())
		}
		s.auditEnd(op.opType, op.fields, false, auditstore.StatusInterrupted, detail)
		return true
	})
}

// mergeDetail returns base plus extra without mutating base, so one operation's
// START and END details can share a base map.
func mergeDetail(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// isUUID accepts the canonical 8-4-4-4-12 hex form control-api parses.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
