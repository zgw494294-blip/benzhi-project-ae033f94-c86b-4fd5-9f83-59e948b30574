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

type validationHistoryEntry struct {
	version uint64
	items   []domain.ValidationBatch
}

func (s *Service) ListValidationBatches(ctx context.Context, caseID string, limit int) (*ValidationBatchList, error) {
	if limit <= 0 || limit > 100 {
		return nil, Invalid("INVALID_RANGE", "limit 必须在 1 至 100 之间")
	}
	c, err := s.repo.ViewCase(ctx, caseID)
	if err != nil {
		return nil, classify(err)
	}
	if cached, ok := s.history.Load(caseID); ok && cached.(validationHistoryEntry).version == c.Version {
		items := cloneValidationBatches(cached.(validationHistoryEntry).items)
		if len(items) > limit {
			items = items[:limit]
		}
		return &ValidationBatchList{Items: items}, nil
	}
	items := cloneValidationBatches(c.ValidationBatches)
	sort.Slice(items, func(i, j int) bool {
		if items[i].CompletedAt.Equal(items[j].CompletedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CompletedAt.After(items[j].CompletedAt)
	})
	s.history.Store(caseID, validationHistoryEntry{version: c.Version, items: cloneValidationBatches(items)})
	if len(items) > limit {
		items = items[:limit]
	}
	return &ValidationBatchList{Items: items}, nil
}

// cloneValidationBatches returns a fully independent copy of the given batches
// so that nested mutable fields (ChangedRefs, InputRevisions, Issues and
// Issues[].AffectedRefs) are isolated from the source and from any cached
// slice. Callers can mutate the returned value without affecting the
// underlying history or subsequent queries.
func cloneValidationBatches(in []domain.ValidationBatch) []domain.ValidationBatch {
	if in == nil {
		return nil
	}
	out := make([]domain.ValidationBatch, len(in))
	for i := range in {
		b := in[i]
		b.ChangedRefs = cloneStrings(in[i].ChangedRefs)
		b.InputRevisions = cloneRevisions(in[i].InputRevisions)
		b.Issues = make([]domain.ValidationBatchIssue, len(in[i].Issues))
		for j := range in[i].Issues {
			issue := in[i].Issues[j]
			issue.AffectedRefs = cloneStrings(in[i].Issues[j].AffectedRefs)
			b.Issues[j] = issue
		}
		out[i] = b
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneRevisions(in map[string]uint32) map[string]uint32 {
	if in == nil {
		return nil
	}
	out := make(map[string]uint32, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Service) DiffValidationBatches(ctx context.Context, caseID, fromID, toID string) (*ValidationBatchDiff, error) {
	if fromID == "" || toID == "" || fromID == toID {
		return nil, Invalid("INVALID_BATCH_RANGE", "必须提供两个不同的校验批次")
	}
	from, err := s.repo.FindValidationBatch(ctx, fromID)
	if err != nil {
		return nil, classify(err)
	}
	to, err := s.repo.FindValidationBatch(ctx, toID)
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
