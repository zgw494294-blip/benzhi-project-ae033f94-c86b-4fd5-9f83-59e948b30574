package audit

import (
	"fmt"
	"stage-rigging-clearance/internal/domain"
)

type ChainError struct {
	Index    int
	Sequence uint64
	Reason   string
}

func (e *ChainError) Error() string {
	return fmt.Sprintf("审计链在索引 %d（序号 %d）损坏：%s", e.Index, e.Sequence, e.Reason)
}
func VerifyChain(events []domain.AuditEvent, from int, d Digester) error {
	if from < 0 || from > len(events) {
		return fmt.Errorf("无效校验断点")
	}
	previous := ""
	if from > 0 {
		previous = events[from-1].Digest
	}
	for i := from; i < len(events); i++ {
		e := events[i]
		if i == 0 && e.Sequence != 1 {
			return &ChainError{i, e.Sequence, "首个序号必须为 1"}
		}
		if e.PreviousDigest != previous {
			return &ChainError{i, e.Sequence, "前序摘要不匹配"}
		}
		want := d.EventDigest(previous, e.CaseID, e.AggregateVersion, e.Kind, e.Payload)
		if e.Digest != want {
			return &ChainError{i, e.Sequence, "事件摘要不匹配"}
		}
		if i > 0 && e.Sequence != events[i-1].Sequence+1 {
			return &ChainError{i, e.Sequence, "序号不连续"}
		}
		previous = e.Digest
	}
	return nil
}
