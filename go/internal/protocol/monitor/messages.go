package monitor

import "encoding/json"

const ProtocolVersion = 1

type Envelope struct {
	Type      string          `json:"type"`
	Version   int             `json:"version"`
	RequestID string          `json:"requestId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type Point struct {
	ItemID   string  `json:"itemId"`
	Instance string  `json:"instance,omitempty"`
	Value    float64 `json:"value"`
}

type PointsPayload struct {
	CollectedAt string  `json:"collectedAt"` // RFC3339
	Points      []Point `json:"points"`
	// CompleteFamilies lists metric families whose instances in this batch are a
	// full snapshot (e.g. "disk", "diskio", "net"). Absent families must not be
	// treated as instance deletions — a collector may have failed temporarily.
	CompleteFamilies []string `json:"completeFamilies,omitempty"`
}

type AckPayload struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func Marshal(typ string, requestID string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Envelope{
		Type:      typ,
		Version:   ProtocolVersion,
		RequestID: requestID,
		Payload:   raw,
	})
}

func UnmarshalPayload[T any](env Envelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}
	err := json.Unmarshal(env.Payload, &out)
	return out, err
}
