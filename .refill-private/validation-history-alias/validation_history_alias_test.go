package validation_history_alias_test

import (
	"context"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
	"testing"
	"time"
)

type historyRepository struct {
	application.Repository
	views int
}

func (r *historyRepository) ViewCase(context.Context, string) (*domain.RiggingCase, error) {
	r.views++
	completed := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	return &domain.RiggingCase{
		ID:      "case-history",
		Version: 7,
		ValidationBatches: []domain.ValidationBatch{{
			ID:               "batch-1",
			CaseID:           "case-history",
			AggregateVersion: 6,
			InputRevisions:   map[string]uint32{"point-1": 3},
			CompletedAt:      completed,
			ChangedRefs:      []string{"point-1"},
			Issues: []domain.ValidationBatchIssue{{
				Identity:     "capacity:point-1",
				AffectedRefs: []string{"point-1"},
				State:        domain.IssueNew,
			}},
		}},
	}, nil
}

func TestValidationHistoryCacheDoesNotLeakCallerMutation(t *testing.T) {
	repo := &historyRepository{}
	service := application.NewService(repo, nil, nil, nil)

	first, err := service.ListValidationBatches(context.Background(), "case-history", 20)
	if err != nil {
		t.Fatalf("首次查询校验历史: %v", err)
	}
	first.Items[0].ChangedRefs[0] = "caller-mutated-change"
	first.Items[0].InputRevisions["point-1"] = 99
	first.Items[0].Issues[0].AffectedRefs[0] = "caller-mutated-issue"

	second, err := service.ListValidationBatches(context.Background(), "case-history", 20)
	if err != nil {
		t.Fatalf("再次查询校验历史: %v", err)
	}
	batch := second.Items[0]
	if batch.ChangedRefs[0] != "point-1" || batch.InputRevisions["point-1"] != 3 || batch.Issues[0].AffectedRefs[0] != "point-1" {
		t.Fatalf("缓存复用泄漏了前一调用方的修改: changedRefs=%v inputRevisions=%v affectedRefs=%v", batch.ChangedRefs, batch.InputRevisions, batch.Issues[0].AffectedRefs)
	}
	if repo.views != 2 {
		t.Fatalf("测试未经过相同 version 的缓存复用路径: ViewCase 调用次数=%d", repo.views)
	}
}
