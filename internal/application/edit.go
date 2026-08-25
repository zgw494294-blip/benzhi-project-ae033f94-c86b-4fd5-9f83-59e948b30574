package application

import (
	"context"
	"stage-rigging-clearance/internal/domain"
)

type WriteMeta struct {
	ExpectedVersion uint64 `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
}
type LoadPointInput struct {
	WriteMeta
	ID                    string  `json:"id"`
	EquipmentCode         string  `json:"equipmentCode"`
	ParentPointID         *string `json:"parentPointId"`
	RatedCapacityKg       int64   `json:"ratedCapacityKg"`
	StaticLoadKg          int64   `json:"staticLoadKg"`
	DynamicFactorPermille int64   `json:"dynamicFactorPermille"`
}

func (s *Service) UpsertLoadPoint(ctx context.Context, caseID string, in LoadPointInput) (*domain.RiggingCase, error) {
	return s.mutate(ctx, caseID, in.IdempotencyKey, "LOAD_POINT_UPSERTED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		return c.UpsertLoadPoint(domain.LoadPoint{ID: in.ID, EquipmentCode: in.EquipmentCode, ParentPointID: in.ParentPointID, RatedCapacityKg: in.RatedCapacityKg, StaticLoadKg: in.StaticLoadKg, DynamicFactorPermille: in.DynamicFactorPermille})
	})
}

type CueInput struct {
	WriteMeta
	ID                 string   `json:"id"`
	Sequence           int      `json:"sequence"`
	Action             string   `json:"action"`
	EquipmentCodes     []string `json:"equipmentCodes"`
	MutexGroup         string   `json:"mutexGroup"`
	ExpectedDurationMs int64    `json:"expectedDurationMs"`
}

type ChangeSetInput struct {
	WriteMeta
	LoadPoints []LoadPointInput `json:"loadPoints"`
	Cues       []CueInput       `json:"cues"`
}

func (s *Service) ApplyChangeSet(ctx context.Context, caseID string, in ChangeSetInput) (*domain.RiggingCase, error) {
	changes := domain.ChangeSet{LoadPoints: make([]domain.LoadPoint, 0, len(in.LoadPoints)), Cues: make([]domain.SceneCue, 0, len(in.Cues))}
	for _, p := range in.LoadPoints {
		changes.LoadPoints = append(changes.LoadPoints, domain.LoadPoint{ID: p.ID, EquipmentCode: p.EquipmentCode, ParentPointID: p.ParentPointID, RatedCapacityKg: p.RatedCapacityKg, StaticLoadKg: p.StaticLoadKg, DynamicFactorPermille: p.DynamicFactorPermille})
	}
	for _, cue := range in.Cues {
		changes.Cues = append(changes.Cues, domain.SceneCue{ID: cue.ID, Sequence: cue.Sequence, Action: cue.Action, EquipmentCodes: cue.EquipmentCodes, MutexGroup: cue.MutexGroup, ExpectedDurationMs: cue.ExpectedDurationMs})
	}
	return s.mutate(ctx, caseID, in.IdempotencyKey, "CHANGE_SET_APPLIED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		if issues := c.ApplyChangeSet(changes); len(issues) > 0 {
			return InvalidDetails("CHANGE_SET_INVALID", "批量变更集预检未通过", map[string]any{"items": issues})
		}
		return nil
	})
}

func (s *Service) UpsertCue(ctx context.Context, caseID string, in CueInput) (*domain.RiggingCase, error) {
	return s.mutate(ctx, caseID, in.IdempotencyKey, "CUE_UPSERTED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		return c.UpsertCue(domain.SceneCue{ID: in.ID, Sequence: in.Sequence, Action: in.Action, EquipmentCodes: in.EquipmentCodes, MutexGroup: in.MutexGroup, ExpectedDurationMs: in.ExpectedDurationMs})
	})
}
func (s *Service) Validate(ctx context.Context, caseID string, in WriteMeta) (*domain.RiggingCase, error) {
	return s.mutate(ctx, caseID, in.IdempotencyKey, "CASE_VALIDATED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		if err := c.RequireStatus(domain.StatusDraft, domain.StatusRemediating); err != nil {
			return err
		}
		started := s.clock.Now()
		c.RunValidation(s.ids.NewID("batch"), started, s.clock.Now(), func() string { return s.ids.NewID("finding") })
		return nil
	})
}
