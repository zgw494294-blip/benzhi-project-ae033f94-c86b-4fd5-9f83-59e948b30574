package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type Digester struct{}

func NewDigester() *Digester { return &Digester{} }
func (Digester) EventDigest(previous, caseID string, version uint64, kind string, payload []byte) string {
	canonical, _ := json.Marshal(struct {
		Previous string          `json:"previous"`
		CaseID   string          `json:"caseId"`
		Version  uint64          `json:"version"`
		Kind     string          `json:"kind"`
		Payload  json.RawMessage `json:"payload"`
	}{previous, caseID, version, kind, payload})
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
func (Digester) SnapshotDigest(snapshot []byte) string {
	sum := sha256.Sum256(snapshot)
	return hex.EncodeToString(sum[:])
}
