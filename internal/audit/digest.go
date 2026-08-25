package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type Digester struct {
	scratch []byte
}

func NewDigester() *Digester { return &Digester{scratch: make([]byte, 0, 1024)} }
func (d *Digester) EventDigest(previous, caseID string, version uint64, kind string, payload []byte) string {
	canonical, _ := json.Marshal(struct {
		Previous string          `json:"previous"`
		CaseID   string          `json:"caseId"`
		Version  uint64          `json:"version"`
		Kind     string          `json:"kind"`
		Payload  json.RawMessage `json:"payload"`
	}{previous, caseID, version, kind, payload})
	d.scratch = append(d.scratch[:0], canonical...)
	sum := sha256.Sum256(d.scratch)
	return hex.EncodeToString(sum[:])
}
func (d *Digester) SnapshotDigest(snapshot []byte) string {
	d.scratch = append(d.scratch[:0], snapshot...)
	sum := sha256.Sum256(d.scratch)
	return hex.EncodeToString(sum[:])
}
