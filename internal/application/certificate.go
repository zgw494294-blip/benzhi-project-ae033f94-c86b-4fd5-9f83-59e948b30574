package application

import (
	"context"
	"encoding/json"
	"stage-rigging-clearance/internal/domain"
	"strings"
)

type CertificateCheck struct {
	Name   string `json:"name"`
	Valid  bool   `json:"valid"`
	Detail string `json:"detail"`
}

type CertificateLookupReport struct {
	Certificate          *domain.ReleaseCertificate `json:"certificate"`
	ShowName             string                     `json:"showName"`
	ScheduledAt          string                     `json:"scheduledAt"`
	CaseStatus           domain.RiggingStatus       `json:"caseStatus"`
	Checks               []CertificateCheck         `json:"checks"`
	Valid                bool                       `json:"valid"`
	FirstDamagedSequence *uint64                    `json:"firstDamagedSequence,omitempty"`
	FrozenSnapshot       json.RawMessage            `json:"frozenSnapshot,omitempty"`
}

func (s *Service) LookupCertificate(ctx context.Context, serial *uint64, certificateID string) (*CertificateLookupReport, error) {
	certificateID = strings.TrimSpace(certificateID)
	if serial == nil && certificateID == "" {
		return nil, Invalid("LOOKUP_REQUIRED", "必须提供 serial 或 certificateID")
	}
	if serial != nil && *serial == 0 {
		return nil, Invalid("INVALID_SERIAL", "serial 必须是非零数字")
	}
	if serial != nil && certificateID != "" {
		return nil, Invalid("LOOKUP_CONFLICT", "serial 与 certificateID 不能同时提供")
	}
	cert, snapshot, err := s.repo.FindCertificate(ctx, serial, certificateID)
	if err != nil {
		return nil, classify(err)
	}
	c, err := s.repo.ViewCase(ctx, cert.CaseID)
	if err != nil {
		return nil, classify(err)
	}
	events, err := s.repo.ListAudit(ctx, cert.CaseID)
	if err != nil {
		return nil, err
	}
	report := &CertificateLookupReport{Certificate: cert, ShowName: c.ShowName, ScheduledAt: c.ScheduledAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), CaseStatus: c.Status, Valid: true}
	snapshotValid := s.digester.SnapshotDigest(snapshot) == cert.SnapshotDigest
	report.Checks = append(report.Checks, CertificateCheck{"SNAPSHOT_DIGEST", snapshotValid, checkDetail(snapshotValid, "冻结快照摘要一致", "冻结快照摘要不一致")})
	chainValid, damaged := s.verifyTimelineDetailed(events)
	if damaged != nil {
		report.FirstDamagedSequence = damaged
	}
	report.Checks = append(report.Checks, CertificateCheck{"AUDIT_CHAIN", chainValid, checkDetail(chainValid, "完整审计链连续且摘要一致", "审计链首次损坏位置已报告")})
	anchored := false
	for _, event := range events {
		if event.Kind == "CASE_RELEASED" && event.PreviousDigest == cert.AuditHeadDigest {
			anchored = true
			break
		}
	}
	report.Checks = append(report.Checks, CertificateCheck{"RELEASE_ANCHOR", anchored, checkDetail(anchored, "CASE_RELEASED 事件锚点一致", "CASE_RELEASED 事件锚点缺失或不一致")})
	var frozen domain.FrozenSnapshot
	versionValid := json.Unmarshal(snapshot, &frozen) == nil && frozen.Version == cert.FrozenVersion
	report.Checks = append(report.Checks, CertificateCheck{"FROZEN_VERSION", versionValid, checkDetail(versionValid, "凭据版本与冻结快照一致", "凭据版本与冻结快照不一致")})
	statusValid := c.Status == domain.StatusReleased
	report.Checks = append(report.Checks, CertificateCheck{"RELEASE_STATUS", statusValid, checkDetail(statusValid, "聚合处于 RELEASED", "聚合未处于 RELEASED")})
	for _, check := range report.Checks {
		if !check.Valid {
			report.Valid = false
		}
	}
	if report.Valid {
		report.FrozenSnapshot = json.RawMessage(snapshot)
	}
	return report, nil
}

func (s *Service) verifyTimelineDetailed(events []domain.AuditEvent) (bool, *uint64) {
	previous := ""
	var sequence uint64
	for index, event := range events {
		validSequence := (index == 0 && event.Sequence == 1) || (index > 0 && event.Sequence == sequence+1)
		valid := event.PreviousDigest == previous && validSequence && event.Digest == s.digester.EventDigest(previous, event.CaseID, event.AggregateVersion, event.Kind, event.Payload)
		if !valid {
			damaged := event.Sequence
			if event.Sequence == 0 {
				damaged = sequence + 1
			}
			return false, &damaged
		}
		previous, sequence = event.Digest, event.Sequence
	}
	return true, nil
}

func checkDetail(valid bool, success, failure string) string {
	if valid {
		return success
	}
	return failure
}
