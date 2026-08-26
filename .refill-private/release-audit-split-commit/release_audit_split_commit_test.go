package releaseauditsplitcommit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/audit"
	"stage-rigging-clearance/internal/domain"
	"testing"
	"time"
)

var errReleaseAuditUnavailable = errors.New("release audit storage unavailable")

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type sequenceIDs struct{ next int }

func (g *sequenceIDs) NewID(prefix string) string {
	g.next++
	return fmt.Sprintf("%s-%d", prefix, g.next)
}

type rollbackRepository struct {
	caseState        *domain.RiggingCase
	idempotency      map[string][]byte
	events           []domain.AuditEvent
	failReleaseAudit bool
}

func (r *rollbackRepository) WithinTransaction(ctx context.Context, fn func(application.Transaction) error) error {
	tx := &rollbackTransaction{
		caseState:        cloneCase(r.caseState),
		idempotency:      cloneBytesMap(r.idempotency),
		events:           append([]domain.AuditEvent(nil), r.events...),
		failReleaseAudit: r.failReleaseAudit,
	}
	if err := fn(tx); err != nil {
		return err
	}
	r.caseState = tx.caseState
	r.idempotency = tx.idempotency
	r.events = tx.events
	return nil
}

func (r *rollbackRepository) ViewCase(context.Context, string) (*domain.RiggingCase, error) {
	return cloneCase(r.caseState), nil
}
func (r *rollbackRepository) ListCases(context.Context) ([]*domain.RiggingCase, error) {
	return nil, errors.New("unexpected ListCases")
}
func (r *rollbackRepository) ListAudit(context.Context, string) ([]domain.AuditEvent, error) {
	return append([]domain.AuditEvent(nil), r.events...), nil
}
func (r *rollbackRepository) LoadSnapshot(context.Context, string) ([]byte, *domain.ReleaseCertificate, error) {
	return nil, nil, errors.New("unexpected LoadSnapshot")
}
func (r *rollbackRepository) FindValidationBatch(context.Context, string) (*domain.ValidationBatch, error) {
	return nil, errors.New("unexpected FindValidationBatch")
}
func (r *rollbackRepository) FindCertificate(context.Context, *uint64, string) (*domain.ReleaseCertificate, []byte, error) {
	return nil, nil, errors.New("unexpected FindCertificate")
}

type rollbackTransaction struct {
	caseState        *domain.RiggingCase
	idempotency      map[string][]byte
	events           []domain.AuditEvent
	failReleaseAudit bool
}

func (tx *rollbackTransaction) LoadCase(context.Context, string) (*domain.RiggingCase, error) {
	return cloneCase(tx.caseState), nil
}
func (tx *rollbackTransaction) SaveCase(_ context.Context, value *domain.RiggingCase, expected uint64) error {
	if tx.caseState.Version != expected {
		return application.Conflict("VERSION_CONFLICT", "方案版本已变化")
	}
	tx.caseState = cloneCase(value)
	return nil
}
func (tx *rollbackTransaction) CreateCase(context.Context, *domain.RiggingCase) error {
	return errors.New("unexpected CreateCase")
}
func (tx *rollbackTransaction) GetIdempotency(_ context.Context, scope, key string) ([]byte, bool, error) {
	raw, ok := tx.idempotency[scope+"\x00"+key]
	return append([]byte(nil), raw...), ok, nil
}
func (tx *rollbackTransaction) PutIdempotency(_ context.Context, scope, key string, raw []byte) error {
	tx.idempotency[scope+"\x00"+key] = append([]byte(nil), raw...)
	return nil
}
func (tx *rollbackTransaction) AuditHead(context.Context, string) (string, uint64, error) {
	if len(tx.events) == 0 {
		return "", 0, nil
	}
	last := tx.events[len(tx.events)-1]
	return last.Digest, last.Sequence, nil
}
func (tx *rollbackTransaction) AppendAudit(_ context.Context, event domain.AuditEvent) error {
	if tx.failReleaseAudit && event.Kind == "CASE_RELEASED" {
		return errReleaseAuditUnavailable
	}
	tx.events = append(tx.events, event)
	return nil
}
func (tx *rollbackTransaction) NextCertificateSerial(context.Context) (uint64, error) {
	return 1, nil
}
func (tx *rollbackTransaction) SaveCertificate(_ context.Context, cert *domain.ReleaseCertificate, _ []byte) error {
	copy := *cert
	tx.caseState.Certificate = &copy
	return nil
}

func cloneCase(value *domain.RiggingCase) *domain.RiggingCase {
	raw, _ := json.Marshal(value)
	var out domain.RiggingCase
	_ = json.Unmarshal(raw, &out)
	return &out
}

func cloneBytesMap(values map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(values))
	for key, value := range values {
		out[key] = append([]byte(nil), value...)
	}
	return out
}

func readyCase(now time.Time) *domain.RiggingCase {
	finished := now.Add(-time.Minute)
	return &domain.RiggingCase{
		ID: "case-release-atomicity", ShowName: "原子放行测试", VenueLimitKg: 5000,
		Owner: "机械师", ScheduledAt: now.Add(time.Hour), CreatedAt: now.Add(-time.Hour),
		Status: domain.StatusReadyForReview, Version: 7,
		LoadPoints: []domain.LoadPoint{{ID: "point-1", CaseID: "case-release-atomicity", EquipmentCode: "LX-1", RatedCapacityKg: 1000, StaticLoadKg: 100, DynamicFactorPermille: 1000, Revision: 1}},
		Cues:       []domain.SceneCue{{ID: "cue-1", CaseID: "case-release-atomicity", Sequence: 1, Action: "升起", EquipmentCodes: []string{"LX-1"}, ExpectedDurationMs: 1000, Revision: 1}},
		Rehearsals: []domain.RehearsalRun{{ID: "run-1", CaseID: "case-release-atomicity", StartedAt: now.Add(-2 * time.Minute), FinishedAt: &finished, Operator: "操作员", Outcome: domain.OutcomePassed, CueResults: []domain.CueResult{{CueID: "cue-1", Success: true, PeakKg: 100}}}},
	}
}

func TestReleaseAuditFailureRollsBackEntireRelease(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &rollbackRepository{
		caseState: readyCase(now), idempotency: map[string][]byte{}, failReleaseAudit: true,
	}
	service := application.NewService(repo, fixedClock{now}, &sequenceIDs{}, audit.NewDigester())
	input := application.ReleaseInput{WriteMeta: application.WriteMeta{ExpectedVersion: 7, IdempotencyKey: "release-once"}, Reviewer: "安全复核员"}

	_, firstErr := service.Release(ctx, repo.caseState.ID, input)
	stateAfterFailure := cloneCase(repo.caseState)
	_, idempotencyCommitted := repo.idempotency[repo.caseState.ID+"\x00release-once"]
	retryResult, retryErr := service.Release(ctx, repo.caseState.ID, input)

	if !errors.Is(firstErr, errReleaseAuditUnavailable) {
		t.Fatalf("首个放行应返回审计故障，实际为 %v", firstErr)
	}
	if stateAfterFailure.Status != domain.StatusReadyForReview || stateAfterFailure.Certificate != nil || idempotencyCommitted || len(repo.events) != 0 || retryErr == nil || retryResult != nil {
		t.Fatalf("审计追加失败未回滚整个放行：首次状态=%s certificate=%v idempotency=%v events=%d retryResult=%v retryErr=%v", stateAfterFailure.Status, stateAfterFailure.Certificate != nil, idempotencyCommitted, len(repo.events), retryResult != nil, retryErr)
	}
}
