package certificate_event_mismatch_test

import (
	"context"
	"encoding/json"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/audit"
	"stage-rigging-clearance/internal/domain"
	"testing"
	"time"
)

type certificateRepo struct {
	caseValue   *domain.RiggingCase
	certificate *domain.ReleaseCertificate
	snapshot    []byte
	events      []domain.AuditEvent
}

func (r *certificateRepo) WithinTransaction(context.Context, func(application.Transaction) error) error {
	return nil
}
func (r *certificateRepo) ViewCase(context.Context, string) (*domain.RiggingCase, error) {
	return r.caseValue, nil
}
func (r *certificateRepo) ListCases(context.Context) ([]*domain.RiggingCase, error) {
	return []*domain.RiggingCase{r.caseValue}, nil
}
func (r *certificateRepo) ListAudit(context.Context, string) ([]domain.AuditEvent, error) {
	return r.events, nil
}
func (r *certificateRepo) LoadSnapshot(context.Context, string) ([]byte, *domain.ReleaseCertificate, error) {
	return r.snapshot, r.certificate, nil
}
func (r *certificateRepo) FindValidationBatch(context.Context, string) (*domain.ValidationBatch, error) {
	return nil, application.NotFound("校验批次不存在")
}
func (r *certificateRepo) FindCertificate(context.Context, *uint64, string) (*domain.ReleaseCertificate, []byte, error) {
	return r.certificate, r.snapshot, nil
}

func TestCertificateLookupRejectsEventPayloadMismatch(t *testing.T) {
	digester := audit.NewDigester()
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	snapshot, err := json.Marshal(domain.FrozenSnapshot{CaseID: "case-cert", Version: 7, Status: domain.StatusReleased})
	if err != nil {
		t.Fatal(err)
	}
	createdPayload := []byte(`{"showName":"凭据演出"}`)
	created := domain.AuditEvent{Sequence: 1, CaseID: "case-cert", Kind: "CASE_CREATED", AggregateVersion: 1, Payload: createdPayload, OccurredAt: now}
	created.Digest = digester.EventDigest("", created.CaseID, created.AggregateVersion, created.Kind, created.Payload)
	original := domain.ReleaseCertificate{
		ID:              "cert-signed",
		CaseID:          "case-cert",
		Serial:          42,
		FrozenVersion:   7,
		SnapshotDigest:  digester.SnapshotDigest(snapshot),
		AuditHeadDigest: created.Digest,
		Reviewer:        "原复核员",
		IssuedAt:        now.Add(time.Minute),
	}
	releasePayload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	released := domain.AuditEvent{Sequence: 2, CaseID: original.CaseID, Kind: "CASE_RELEASED", AggregateVersion: 7, Payload: releasePayload, PreviousDigest: created.Digest, OccurredAt: original.IssuedAt}
	released.Digest = digester.EventDigest(released.PreviousDigest, released.CaseID, released.AggregateVersion, released.Kind, released.Payload)

	tampered := original
	tampered.Reviewer = "被替换的复核员"
	repo := &certificateRepo{
		caseValue:   &domain.RiggingCase{ID: original.CaseID, ShowName: "凭据演出", ScheduledAt: now.Add(time.Hour), Status: domain.StatusReleased, Certificate: &tampered},
		certificate: &tampered,
		snapshot:    snapshot,
		events:      []domain.AuditEvent{created, released},
	}
	service := application.NewService(repo, nil, nil, digester)
	report, err := service.LookupCertificate(context.Background(), &tampered.Serial, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid {
		t.Fatal("凭据记录与已摘要的 CASE_RELEASED 载荷不一致时仍被判定有效")
	}
}
