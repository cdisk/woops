package sessioncore

// BaseClaims are the fields common to every session ticket type.
type BaseClaims struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"`
	AssetID   string `json:"assetId"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
}
