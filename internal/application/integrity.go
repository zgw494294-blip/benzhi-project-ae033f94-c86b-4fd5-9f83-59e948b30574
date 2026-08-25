package application

import (
	"context"
	"stage-rigging-clearance/internal/domain"
)

type IntegrityReport struct {
	CaseID              string                     `json:"caseId"`
	EventsChecked       int                        `json:"eventsChecked"`
	AuditHeadDigest     string                     `json:"auditHeadDigest"`
	AuditChainValid     bool                       `json:"auditChainValid"`
	FirstDamagedIndex   *int                       `json:"firstDamagedIndex,omitempty"`
	SnapshotValid       *bool                      `json:"snapshotValid,omitempty"`
	Certificate         *domain.ReleaseCertificate `json:"certificate,omitempty"`
	CertificateAnchored *bool                      `json:"certificateAnchored,omitempty"`
}

func (s *Service) VerifyIntegrity(ctx context.Context, caseID string, from int) (*IntegrityReport, error) {
	if from < 0 {
		return nil, Invalid("INVALID_BREAKPOINT", "审计校验断点不能为负数")
	}
	c, err := s.repo.ViewCase(ctx, caseID)
	if err != nil {
		return nil, classify(err)
	}
	events, err := s.repo.ListAudit(context.WithoutCancel(ctx), caseID)
	if err != nil {
		return nil, err
	}
	if from > len(events) {
		return nil, Invalid("INVALID_BREAKPOINT", "审计校验断点超过事件数量")
	}
	report := &IntegrityReport{CaseID: caseID, EventsChecked: len(events) - from, AuditChainValid: true, Certificate: c.Certificate}
	previous := ""
	var priorSequence uint64
	if from > 0 {
		previous = events[from-1].Digest
		priorSequence = events[from-1].Sequence
	}
	for index := from; index < len(events); index++ {
		event := events[index]
		validSequence := (index == 0 && event.Sequence == 1) || (index > 0 && event.Sequence == priorSequence+1)
		validDigest := event.PreviousDigest == previous && event.Digest == s.digester.EventDigest(previous, event.CaseID, event.AggregateVersion, event.Kind, event.Payload)
		if !validSequence || !validDigest {
			damaged := index
			report.FirstDamagedIndex = &damaged
			report.AuditChainValid = false
			break
		}
		previous, priorSequence = event.Digest, event.Sequence
	}
	if len(events) > 0 {
		report.AuditHeadDigest = events[len(events)-1].Digest
	}
	if c.Certificate != nil {
		snapshot, cert, err := s.repo.LoadSnapshot(context.WithoutCancel(ctx), caseID)
		if err != nil {
			return nil, err
		}
		valid := cert != nil && s.digester.SnapshotDigest(snapshot) == cert.SnapshotDigest
		report.SnapshotValid = &valid
		anchored := false
		for _, event := range events {
			if event.Kind == "CASE_RELEASED" && event.PreviousDigest == cert.AuditHeadDigest {
				anchored = true
				break
			}
		}
		report.CertificateAnchored = &anchored
	}
	return report, nil
}
