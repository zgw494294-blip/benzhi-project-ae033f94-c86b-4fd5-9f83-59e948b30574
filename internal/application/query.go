package application

import (
	"context"
	"encoding/json"
	"stage-rigging-clearance/internal/domain"
)

type CaseView struct {
	Case              *domain.RiggingCase `json:"case"`
	Timeline          []domain.AuditEvent `json:"timeline"`
	SnapshotValid     *bool               `json:"snapshotValid,omitempty"`
	AuditChainValid   bool                `json:"auditChainValid"`
	FrozenSnapshot    json.RawMessage     `json:"frozenSnapshot,omitempty"`
	RehearsalProgress *RehearsalProgress  `json:"rehearsalProgress,omitempty"`
}

type RehearsalProgress struct {
	RunID         string             `json:"runId"`
	RecordedCount int                `json:"recordedCount"`
	TotalCount    int                `json:"totalCount"`
	Recorded      []domain.CueResult `json:"recorded"`
	PendingCueIDs []string           `json:"pendingCueIds"`
}

func (s *Service) GetCase(ctx context.Context, id string) (*CaseView, error) {
	c, err := s.repo.ViewCase(ctx, id)
	if err != nil {
		return nil, classify(err)
	}
	events, err := s.repo.ListAudit(ctx, id)
	if err != nil {
		return nil, err
	}
	v := &CaseView{Case: c, Timeline: events, AuditChainValid: s.verifyTimeline(events)}
	if len(c.Rehearsals) > 0 {
		run := c.Rehearsals[len(c.Rehearsals)-1]
		if run.Outcome == domain.OutcomePending {
			recorded := map[string]bool{}
			for _, result := range run.CueResults {
				recorded[result.CueID] = true
			}
			progress := &RehearsalProgress{RunID: run.ID, RecordedCount: len(run.CueResults), TotalCount: len(c.Cues), Recorded: append([]domain.CueResult(nil), run.CueResults...), PendingCueIDs: []string{}}
			for _, cue := range c.Cues {
				if !recorded[cue.ID] {
					progress.PendingCueIDs = append(progress.PendingCueIDs, cue.ID)
				}
			}
			v.RehearsalProgress = progress
		}
	}
	if c.Certificate != nil {
		snapshot, cert, err := s.repo.LoadSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		ok := cert != nil && s.digester.SnapshotDigest(snapshot) == cert.SnapshotDigest
		v.SnapshotValid = &ok
		if ok {
			v.FrozenSnapshot = json.RawMessage(snapshot)
		}
	}
	return v, nil
}
func (s *Service) ListCases(ctx context.Context) ([]*domain.RiggingCase, error) {
	return s.repo.ListCases(ctx)
}
func (s *Service) VerifyCertificate(ctx context.Context, id string) (bool, error) {
	snapshot, cert, err := s.repo.LoadSnapshot(ctx, id)
	if err != nil {
		return false, err
	}
	if cert == nil {
		return false, NotFound("放行凭据不存在")
	}
	events, err := s.repo.ListAudit(ctx, id)
	if err != nil {
		return false, err
	}
	if !s.verifyTimeline(events) || s.digester.SnapshotDigest(snapshot) != cert.SnapshotDigest {
		return false, nil
	}
	for _, event := range events {
		if event.Kind == "CASE_RELEASED" {
			return event.PreviousDigest == cert.AuditHeadDigest, nil
		}
	}
	return false, nil
}

func (s *Service) verifyTimeline(events []domain.AuditEvent) bool {
	previous := ""
	var sequence uint64
	for index, event := range events {
		if event.PreviousDigest != previous || (index == 0 && event.Sequence != 1) || (index > 0 && event.Sequence != sequence+1) {
			return false
		}
		want := s.digester.EventDigest(previous, event.CaseID, event.AggregateVersion, event.Kind, event.Payload)
		if want != event.Digest {
			return false
		}
		previous, sequence = event.Digest, event.Sequence
	}
	return true
}
