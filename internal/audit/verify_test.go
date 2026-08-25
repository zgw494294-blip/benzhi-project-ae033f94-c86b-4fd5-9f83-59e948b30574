package audit

import (
	"stage-rigging-clearance/internal/domain"
	"testing"
)

func TestVerifyChainReportsFirstDamage(t *testing.T) {
	d := NewDigester()
	first := domain.AuditEvent{Sequence: 1, CaseID: "case", Kind: "CREATED", AggregateVersion: 1, Payload: []byte(`{"ok":true}`)}
	first.Digest = d.EventDigest("", first.CaseID, first.AggregateVersion, first.Kind, first.Payload)
	second := domain.AuditEvent{Sequence: 2, CaseID: "case", Kind: "EDITED", AggregateVersion: 2, Payload: []byte(`{"point":"P1"}`), PreviousDigest: first.Digest}
	second.Digest = d.EventDigest(second.PreviousDigest, second.CaseID, second.AggregateVersion, second.Kind, second.Payload)
	events := []domain.AuditEvent{first, second}
	if err := VerifyChain(events, 0, *d); err != nil {
		t.Fatal(err)
	}
	events[1].Payload = []byte(`{"point":"P2"}`)
	err := VerifyChain(events, 0, *d)
	damaged, ok := err.(*ChainError)
	if !ok || damaged.Index != 1 {
		t.Fatalf("未定位首个损坏位置: %v", err)
	}
}
