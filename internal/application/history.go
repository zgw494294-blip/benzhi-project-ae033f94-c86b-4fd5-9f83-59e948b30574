package application

import (
	"context"
	"sort"
	"stage-rigging-clearance/internal/domain"
)

type ValidationBatchList struct {
	Items []domain.ValidationBatch `json:"items"`
}

type ValidationBatchDiff struct {
	CaseID               string   `json:"caseId"`
	FromBatchID          string   `json:"fromBatchId"`
	ToBatchID            string   `json:"toBatchId"`
	NewIdentities        []string `json:"newIdentities"`
	PersistentIdentities []string `json:"persistentIdentities"`
	ResolvedIdentities   []string `json:"resolvedIdentities"`
}

func (s *Service) ListValidationBatches(ctx context.Context, caseID string, limit int) (*ValidationBatchList, error) {
	if limit <= 0 || limit > 100 {
		return nil, Invalid("INVALID_RANGE", "limit 必须在 1 至 100 之间")
	}
	c, err := s.repo.ViewCase(ctx, caseID)
	if err != nil {
		return nil, classify(err)
	}
	items := append([]domain.ValidationBatch(nil), c.ValidationBatches...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].CompletedAt.Equal(items[j].CompletedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CompletedAt.After(items[j].CompletedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return &ValidationBatchList{Items: items}, nil
}

func (s *Service) DiffValidationBatches(ctx context.Context, caseID, fromID, toID string) (*ValidationBatchDiff, error) {
	if fromID == "" || toID == "" || fromID == toID {
		return nil, Invalid("INVALID_BATCH_RANGE", "必须提供两个不同的校验批次")
	}
	from, err := s.repo.FindValidationBatch(ctx, fromID)
	if err != nil {
		return nil, classify(err)
	}
	to, err := s.repo.FindValidationBatch(context.WithoutCancel(ctx), toID)
	if err != nil {
		return nil, classify(err)
	}
	if from.CaseID != caseID || to.CaseID != caseID {
		return nil, Invalid("BATCH_SCOPE_MISMATCH", "校验批次不属于当前方案")
	}
	fromSet, toSet := currentBatchIdentities(*from), currentBatchIdentities(*to)
	result := &ValidationBatchDiff{CaseID: caseID, FromBatchID: fromID, ToBatchID: toID, NewIdentities: []string{}, PersistentIdentities: []string{}, ResolvedIdentities: []string{}}
	for id := range toSet {
		if fromSet[id] {
			result.PersistentIdentities = append(result.PersistentIdentities, id)
		} else {
			result.NewIdentities = append(result.NewIdentities, id)
		}
	}
	for id := range fromSet {
		if !toSet[id] {
			result.ResolvedIdentities = append(result.ResolvedIdentities, id)
		}
	}
	sort.Strings(result.NewIdentities)
	sort.Strings(result.PersistentIdentities)
	sort.Strings(result.ResolvedIdentities)
	return result, nil
}

func currentBatchIdentities(batch domain.ValidationBatch) map[string]bool {
	out := map[string]bool{}
	for _, issue := range batch.Issues {
		if issue.State != domain.IssueResolved {
			out[issue.Identity] = true
		}
	}
	return out
}
