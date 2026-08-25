package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"stage-rigging-clearance/internal/domain"
)

type Service struct {
	repo     Repository
	clock    Clock
	ids      IDGenerator
	digester Digester
}

type idempotentResult struct {
	Operation   string              `json:"operation"`
	Fingerprint string              `json:"fingerprint"`
	Case        *domain.RiggingCase `json:"case"`
	Failure     *AppError           `json:"failure,omitempty"`
}

func NewService(repo Repository, clock Clock, ids IDGenerator, digester Digester) *Service {
	return &Service{repo: repo, clock: clock, ids: ids, digester: digester}
}

func (s *Service) mutate(ctx context.Context, caseID, key, operation, fingerprint string, expected uint64, fn func(*domain.RiggingCase) error) (*domain.RiggingCase, error) {
	if key == "" {
		return nil, Invalid("IDEMPOTENCY_REQUIRED", "idempotencyKey 不能为空")
	}
	var result *domain.RiggingCase
	var committedConflict error
	err := s.repo.WithinTransaction(ctx, func(tx Transaction) error {
		if raw, ok, err := tx.GetIdempotency(ctx, caseID, key); err != nil {
			return err
		} else if ok {
			var cached idempotentResult
			if json.Unmarshal(raw, &cached) != nil || cached.Case == nil {
				return errors.New("幂等响应损坏")
			}
			if cached.Operation != operation || cached.Fingerprint != fingerprint {
				current, err := tx.LoadCase(ctx, caseID)
				if err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{"code": "IDEMPOTENCY_CONFLICT", "attemptedOperation": operation, "idempotencyKey": key})
				if err := s.appendEvent(ctx, tx, current, "WRITE_CONFLICT", payload); err != nil {
					return err
				}
				committedConflict = Conflict("IDEMPOTENCY_CONFLICT", "idempotencyKey 已用于不同的写请求")
				return nil
			}
			result = cached.Case
			return nil
		}
		c, err := tx.LoadCase(ctx, caseID)
		if err != nil {
			return err
		}
		if c.Version != expected {
			payload, _ := json.Marshal(map[string]any{"code": "VERSION_CONFLICT", "attemptedOperation": operation, "expectedVersion": expected, "actualVersion": c.Version})
			if err := s.appendEvent(ctx, tx, c, "WRITE_CONFLICT", payload); err != nil {
				return err
			}
			committedConflict = Conflict("VERSION_CONFLICT", fmt.Sprintf("expectedVersion=%d，当前 version=%d", expected, c.Version))
			return nil
		}
		before := c.Version
		previousBatchStale := true
		if len(c.ValidationBatches) > 0 {
			previousBatchStale = c.ValidationBatches[len(c.ValidationBatches)-1].Stale
		}
		if err := fn(c); err != nil {
			return classify(err)
		}
		if err := tx.SaveCase(ctx, c, before); err != nil {
			return err
		}
		auditSummary := map[string]any{"operation": operation, "status": c.Status}
		if operation == "CASE_VALIDATED" && len(c.ValidationBatches) > 0 {
			batch := c.ValidationBatches[len(c.ValidationBatches)-1]
			counts := map[domain.ValidationIssueState]int{}
			rules := make([]map[string]any, 0, len(batch.Issues))
			for _, issue := range batch.Issues {
				counts[issue.State]++
				if issue.State != domain.IssueResolved {
					rules = append(rules, map[string]any{"ruleCode": issue.RuleCode, "severity": issue.Severity, "affectedRefs": issue.AffectedRefs, "calculation": issue.Message})
				}
			}
			auditSummary["batchId"], auditSummary["aggregateVersion"], auditSummary["issueCounts"], auditSummary["rules"] = batch.ID, batch.AggregateVersion, counts, rules
		}
		if operation == "REHEARSAL_CUE_SAVED" && len(c.Rehearsals) > 0 {
			run := c.Rehearsals[len(c.Rehearsals)-1]
			auditSummary["runId"], auditSummary["recorded"], auditSummary["total"] = run.ID, len(run.CueResults), len(c.Cues)
		}
		if len(c.ValidationBatches) > 0 {
			latest := c.ValidationBatches[len(c.ValidationBatches)-1]
			if latest.Stale {
				auditSummary["staleBatchId"], auditSummary["changedRefs"] = latest.ID, latest.ChangedRefs
			}
		}
		payload, _ := json.Marshal(auditSummary)
		if err := s.appendEvent(ctx, tx, c, operation, payload); err != nil {
			return err
		}
		if len(c.ValidationBatches) > 0 {
			latest := c.ValidationBatches[len(c.ValidationBatches)-1]
			if !previousBatchStale && latest.Stale {
				stalePayload, _ := json.Marshal(map[string]any{"batchId": latest.ID, "changedRefs": latest.ChangedRefs})
				if err := s.appendEvent(ctx, tx, c, "VALIDATION_BATCH_STALE", stalePayload); err != nil {
					return err
				}
			}
		}
		raw, _ := json.Marshal(idempotentResult{Operation: operation, Fingerprint: fingerprint, Case: c})
		if err := tx.PutIdempotency(ctx, caseID, key, raw); err != nil {
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

func (s *Service) fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	return s.digester.SnapshotDigest(raw)
}
func (s *Service) appendEvent(ctx context.Context, tx Transaction, c *domain.RiggingCase, kind string, payload []byte) error {
	head, seq, err := tx.AuditHead(ctx, c.ID)
	if err != nil {
		return err
	}
	e := domain.AuditEvent{Sequence: seq + 1, CaseID: c.ID, Kind: kind, AggregateVersion: c.Version, Payload: payload, PreviousDigest: head, OccurredAt: s.clock.Now().UTC()}
	e.Digest = s.digester.EventDigest(head, c.ID, c.Version, kind, payload)
	return tx.AppendAudit(ctx, e)
}
func classify(err error) error {
	if err == nil {
		return nil
	}
	var app *AppError
	if errors.As(err, &app) {
		return err
	}
	var v domain.Violation
	if errors.As(err, &v) {
		return InvalidDetails(v.Code, v.Message, v.Details)
	}
	return err
}
