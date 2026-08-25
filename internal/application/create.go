package application

import (
	"context"
	"encoding/json"
	"stage-rigging-clearance/internal/domain"
	"time"
)

type CreateCaseInput struct {
	ShowName       string    `json:"showName"`
	VenueLimitKg   int64     `json:"venueLimitKg"`
	Owner          string    `json:"owner"`
	ScheduledAt    time.Time `json:"scheduledAt"`
	IdempotencyKey string    `json:"idempotencyKey"`
}

func (s *Service) CreateCase(ctx context.Context, in CreateCaseInput) (*domain.RiggingCase, error) {
	if in.IdempotencyKey == "" {
		return nil, Invalid("IDEMPOTENCY_REQUIRED", "idempotencyKey 不能为空")
	}
	id := s.ids.NewID("case")
	fingerprint := s.fingerprint(in)
	var result *domain.RiggingCase
	var committedConflict error
	err := s.repo.WithinTransaction(ctx, func(tx Transaction) error {
		if raw, ok, err := tx.GetIdempotency(ctx, "create", in.IdempotencyKey); err != nil {
			return err
		} else if ok {
			var cached idempotentResult
			if json.Unmarshal(raw, &cached) != nil || cached.Case == nil {
				return Invalid("IDEMPOTENCY_CORRUPT", "幂等结果无法读取")
			}
			if cached.Operation != "CASE_CREATED" || cached.Fingerprint != fingerprint {
				current, err := tx.LoadCase(ctx, cached.Case.ID)
				if err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{"code": "IDEMPOTENCY_CONFLICT", "attemptedOperation": "CASE_CREATED", "idempotencyKey": in.IdempotencyKey})
				if err := s.appendEvent(ctx, tx, current, "WRITE_CONFLICT", payload); err != nil {
					return err
				}
				committedConflict = Conflict("IDEMPOTENCY_CONFLICT", "idempotencyKey 已用于不同的创建请求")
				return nil
			}
			result = cached.Case
			return nil
		}
		c, err := domain.NewCase(id, in.ShowName, in.VenueLimitKg, in.Owner, in.ScheduledAt, s.clock.Now())
		if err != nil {
			return classify(err)
		}
		if err := tx.CreateCase(ctx, c); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"showName": c.ShowName, "owner": c.Owner})
		if err := s.appendEvent(ctx, tx, c, "CASE_CREATED", payload); err != nil {
			return err
		}
		raw, _ := json.Marshal(idempotentResult{Operation: "CASE_CREATED", Fingerprint: fingerprint, Case: c})
		if err := tx.PutIdempotency(ctx, "create", in.IdempotencyKey, raw); err != nil {
			return err
		}
		result = c
		return nil
	})
	if err != nil {
		return nil, classify(err)
	}
	if committedConflict != nil {
		return nil, committedConflict
	}
	return result, nil
}
