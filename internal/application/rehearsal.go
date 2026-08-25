package application

import (
	"context"
	"encoding/json"
	"errors"
	"stage-rigging-clearance/internal/domain"
	"strings"
)

type StartRehearsalInput struct {
	WriteMeta
	Operator string `json:"operator"`
}

func (s *Service) StartRehearsal(ctx context.Context, caseID string, in StartRehearsalInput) (*domain.RiggingCase, error) {
	return s.mutate(ctx, caseID, in.IdempotencyKey, "REHEARSAL_STARTED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		_, err := c.StartRehearsal(s.ids.NewID("run"), in.Operator, s.clock.Now())
		return err
	})
}

type CompleteRehearsalInput struct {
	WriteMeta
	RunID   string             `json:"runId"`
	Results []domain.CueResult `json:"results"`
}

type SaveCueResultInput struct {
	WriteMeta
	RunID     string `json:"runId"`
	CueID     string `json:"cueId"`
	Success   bool   `json:"success"`
	PeakKg    int64  `json:"peakKg"`
	Deviation string `json:"deviation"`
	Evidence  string `json:"evidence"`
	Operator  string `json:"operator"`
}

func (s *Service) SaveCueResult(ctx context.Context, caseID string, in SaveCueResultInput) (*domain.RiggingCase, error) {
	return s.mutate(ctx, caseID, in.IdempotencyKey, "REHEARSAL_CUE_SAVED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		return c.SaveCueResult(in.RunID, domain.CueResult{CueID: in.CueID, Success: in.Success, PeakKg: in.PeakKg, Deviation: in.Deviation, Evidence: in.Evidence}, in.Operator, s.clock.Now())
	})
}

func (s *Service) CompleteRehearsal(ctx context.Context, caseID string, in CompleteRehearsalInput) (*domain.RiggingCase, error) {
	return s.mutate(ctx, caseID, in.IdempotencyKey, "REHEARSAL_COMPLETED", s.fingerprint(in), in.ExpectedVersion, func(c *domain.RiggingCase) error {
		return c.CompleteRehearsal(in.RunID, in.Results, s.clock.Now(), func() string { return s.ids.NewID("finding") })
	})
}

type RemediationInput struct {
	WriteMeta
	FindingID    string            `json:"findingId"`
	Revision     uint32            `json:"revision"`
	Note         string            `json:"note"`
	SubmittedBy  string            `json:"submittedBy"`
	RecheckInput *domain.CueResult `json:"recheckInput,omitempty"`
}

func (s *Service) Remediate(ctx context.Context, caseID string, in RemediationInput) (*domain.RiggingCase, error) {
	if in.IdempotencyKey == "" {
		return nil, Invalid("IDEMPOTENCY_REQUIRED", "idempotencyKey 不能为空")
	}
	var result *domain.RiggingCase
	var committedError error
	fingerprint := s.fingerprint(in)
	err := s.repo.WithinTransaction(ctx, func(tx Transaction) error {
		if raw, ok, err := tx.GetIdempotency(ctx, caseID, in.IdempotencyKey); err != nil {
			return err
		} else if ok {
			var cached idempotentResult
			if json.Unmarshal(raw, &cached) != nil || cached.Case == nil {
				return errors.New("幂等响应损坏")
			}
			if cached.Operation != "FINDING_REMEDIATED" || cached.Fingerprint != fingerprint {
				current, err := tx.LoadCase(ctx, caseID)
				if err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{"code": "IDEMPOTENCY_CONFLICT", "attemptedOperation": "FINDING_REMEDIATED", "idempotencyKey": in.IdempotencyKey})
				if err := s.appendEvent(ctx, tx, current, "WRITE_CONFLICT", payload); err != nil {
					return err
				}
				committedError = Conflict("IDEMPOTENCY_CONFLICT", "idempotencyKey 已用于不同的整改请求")
				return nil
			}
			result = cached.Case
			if cached.Failure != nil {
				committedError = cached.Failure
			}
			return nil
		}
		c, err := tx.LoadCase(ctx, caseID)
		if err != nil {
			return err
		}
		if c.Version != in.ExpectedVersion {
			payload, _ := json.Marshal(map[string]any{"code": "VERSION_CONFLICT", "attemptedOperation": "FINDING_REMEDIATED", "expectedVersion": in.ExpectedVersion, "actualVersion": c.Version})
			if err := s.appendEvent(ctx, tx, c, "WRITE_CONFLICT", payload); err != nil {
				return err
			}
			committedError = Conflict("VERSION_CONFLICT", "方案版本已变化")
			return nil
		}
		before := c.Version
		attempt, err := c.AttemptRemediation(s.ids.NewID("attempt"), domain.RemediationRequest{FindingID: in.FindingID, Revision: in.Revision, Note: in.Note, SubmittedBy: in.SubmittedBy, RecheckInput: in.RecheckInput}, s.clock.Now())
		if err != nil {
			return classify(err)
		}
		if err := tx.SaveCase(ctx, c, before); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"findingId": in.FindingID, "attemptId": attempt.ID, "source": attempt.RecheckType, "passed": attempt.Passed, "conclusion": attempt.Conclusion})
		if err := s.appendEvent(ctx, tx, c, "FINDING_REMEDIATED", payload); err != nil {
			return err
		}
		var failure *AppError
		if !attempt.Passed {
			failure = &AppError{Kind: "validation", Code: remediationFailureCode(attempt), Message: attempt.Conclusion, Details: map[string]any{"attemptId": attempt.ID}}
			committedError = failure
		}
		raw, _ := json.Marshal(idempotentResult{Operation: "FINDING_REMEDIATED", Fingerprint: fingerprint, Case: c, Failure: failure})
		if err := tx.PutIdempotency(ctx, caseID, in.IdempotencyKey, raw); err != nil {
			return err
		}
		result = c
		return nil
	})
	if err != nil {
		return nil, classify(err)
	}
	if committedError != nil {
		return nil, committedError
	}
	return result, nil
}

func remediationFailureCode(attempt *domain.RemediationAttempt) string {
	if attempt.RecheckType == domain.FindingRehearsal && attempt.RecheckInput == nil {
		return "REHEARSAL_RETEST_REQUIRED"
	}
	if attempt.RecheckType == domain.FindingValidation && strings.Contains(attempt.Conclusion, "revision") {
		return "UNTRACEABLE_REMEDIATION"
	}
	if attempt.RecheckType == domain.FindingRehearsal && strings.Contains(attempt.Conclusion, "证据") {
		return "EVIDENCE_REQUIRED"
	}
	if attempt.RecheckType == domain.FindingRehearsal && strings.Contains(attempt.Conclusion, "影响范围") {
		return "CUE_SCOPE_MISMATCH"
	}
	return "RECHECK_FAILED"
}
