package application

import (
	"context"
	"encoding/json"
	"stage-rigging-clearance/internal/domain"
)

type ReleaseInput struct {
	WriteMeta
	Reviewer string `json:"reviewer"`
}

func (s *Service) Release(ctx context.Context, caseID string, in ReleaseInput) (*domain.RiggingCase, error) {
	if in.IdempotencyKey == "" {
		return nil, Invalid("IDEMPOTENCY_REQUIRED", "idempotencyKey 不能为空")
	}
	var result *domain.RiggingCase
	var committedConflict error
	var releaseEventPayload []byte
	var appendReleaseEvent bool
	fingerprint := s.fingerprint(in)
	err := s.repo.WithinTransaction(ctx, func(tx Transaction) error {
		if raw, ok, err := tx.GetIdempotency(ctx, caseID, in.IdempotencyKey); err != nil {
			return err
		} else if ok {
			var cached idempotentResult
			if json.Unmarshal(raw, &cached) != nil || cached.Case == nil {
				return Invalid("IDEMPOTENCY_CORRUPT", "幂等结果损坏")
			}
			if cached.Operation != "CASE_RELEASED" || cached.Fingerprint != fingerprint {
				current, err := tx.LoadCase(ctx, caseID)
				if err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{"code": "IDEMPOTENCY_CONFLICT", "attemptedOperation": "CASE_RELEASED", "idempotencyKey": in.IdempotencyKey})
				if err := s.appendEvent(ctx, tx, current, "WRITE_CONFLICT", payload); err != nil {
					return err
				}
				committedConflict = Conflict("IDEMPOTENCY_CONFLICT", "idempotencyKey 已用于不同的放行请求")
				return nil
			}
			result = cached.Case
			return nil
		}
		c, err := tx.LoadCase(ctx, caseID)
		if err != nil {
			return err
		}
		if c.Version != in.ExpectedVersion {
			payload, _ := json.Marshal(map[string]any{"code": "VERSION_CONFLICT", "attemptedOperation": "CASE_RELEASED", "expectedVersion": in.ExpectedVersion, "actualVersion": c.Version})
			if err := s.appendEvent(ctx, tx, c, "WRITE_CONFLICT", payload); err != nil {
				return err
			}
			committedConflict = Conflict("VERSION_CONFLICT", "方案版本已变化")
			return nil
		}
		before := c.Version
		if err := c.Release(in.Reviewer, s.clock.Now()); err != nil {
			return classify(err)
		}
		// 快照在证书附加前生成，避免摘要自引用。
		snapshot, err := c.FreezeSnapshot()
		if err != nil {
			return err
		}
		snapshotDigest := s.digester.SnapshotDigest(snapshot)
		head, _, err := tx.AuditHead(ctx, c.ID)
		if err != nil {
			return err
		}
		serial, err := tx.NextCertificateSerial(ctx)
		if err != nil {
			return err
		}
		cert := &domain.ReleaseCertificate{ID: s.ids.NewID("cert"), CaseID: c.ID, Serial: serial, FrozenVersion: c.Version, SnapshotDigest: snapshotDigest, AuditHeadDigest: head, Reviewer: in.Reviewer, IssuedAt: s.clock.Now().UTC()}
		c.Certificate = cert
		if err := tx.SaveCase(ctx, c, before); err != nil {
			return err
		}
		if err := tx.SaveCertificate(ctx, cert, snapshot); err != nil {
			return err
		}
		releaseEventPayload, _ = json.Marshal(cert)
		appendReleaseEvent = true
		raw, _ := json.Marshal(idempotentResult{Operation: "CASE_RELEASED", Fingerprint: fingerprint, Case: c})
		if err := tx.PutIdempotency(ctx, caseID, in.IdempotencyKey, raw); err != nil {
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
	if appendReleaseEvent {
		if err := s.repo.WithinTransaction(ctx, func(tx Transaction) error {
			return s.appendEvent(ctx, tx, result, "CASE_RELEASED", releaseEventPayload)
		}); err != nil {
			return nil, classify(err)
		}
	}
	return result, nil
}
