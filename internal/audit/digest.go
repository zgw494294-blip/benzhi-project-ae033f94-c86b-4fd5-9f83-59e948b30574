package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Digester 计算审计事件与快照的 SHA-256 摘要。
// 它不持有可变状态，因此同一实例可被多个 goroutine 并发调用：
// 每次调用都在各自独占的字节切片上计算，结果始终等同于独立计算的 SHA-256。
type Digester struct{}

func NewDigester() *Digester { return &Digester{} }
func (d *Digester) EventDigest(previous, caseID string, version uint64, kind string, payload []byte) string {
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
func (d *Digester) SnapshotDigest(snapshot []byte) string {
	sum := sha256.Sum256(snapshot)
	return hex.EncodeToString(sum[:])
}
