package domain

import (
	"encoding/json"
	"time"
)

type RiggingStatus string

const (
	StatusDraft          RiggingStatus = "DRAFT"
	StatusValidated      RiggingStatus = "VALIDATED"
	StatusRehearsing     RiggingStatus = "REHEARSING"
	StatusRemediating    RiggingStatus = "REMEDIATING"
	StatusReadyForReview RiggingStatus = "READY_FOR_REVIEW"
	StatusReleased       RiggingStatus = "RELEASED"
)

type FindingSource string

const (
	FindingValidation FindingSource = "VALIDATION"
	FindingRehearsal  FindingSource = "REHEARSAL"
)

type Severity string

const (
	SeverityWarning  Severity = "WARNING"
	SeverityBlocking Severity = "BLOCKING"
)

type FindingStatus string

const (
	FindingOpen   FindingStatus = "OPEN"
	FindingClosed FindingStatus = "CLOSED"
)

type RehearsalOutcome string

const (
	OutcomePending RehearsalOutcome = "PENDING"
	OutcomePassed  RehearsalOutcome = "PASSED"
	OutcomeBlocked RehearsalOutcome = "BLOCKED"
)

type LoadPoint struct {
	ID                    string  `json:"id"`
	CaseID                string  `json:"caseId"`
	EquipmentCode         string  `json:"equipmentCode"`
	ParentPointID         *string `json:"parentPointId,omitempty"`
	RatedCapacityKg       int64   `json:"ratedCapacityKg"`
	StaticLoadKg          int64   `json:"staticLoadKg"`
	DynamicFactorPermille int64   `json:"dynamicFactorPermille"`
	Revision              uint32  `json:"revision"`
}
type SceneCue struct {
	ID                 string   `json:"id"`
	CaseID             string   `json:"caseId"`
	Sequence           int      `json:"sequence"`
	Action             string   `json:"action"`
	EquipmentCodes     []string `json:"equipmentCodes"`
	MutexGroup         string   `json:"mutexGroup"`
	ExpectedDurationMs int64    `json:"expectedDurationMs"`
	Revision           uint32   `json:"revision"`
}
type SafetyFinding struct {
	ID                  string            `json:"id"`
	CaseID              string            `json:"caseId"`
	Source              FindingSource     `json:"source"`
	RuleCode            string            `json:"ruleCode"`
	Severity            Severity          `json:"severity"`
	Message             string            `json:"message"`
	AffectedRefs        []string          `json:"affectedRefs"`
	ObservedRevisions   map[string]uint32 `json:"observedRevisions,omitempty"`
	Status              FindingStatus     `json:"status"`
	RemediationRevision *uint32           `json:"remediationRevision,omitempty"`
	ClosedAt            *time.Time        `json:"closedAt,omitempty"`
}
type CueResult struct {
	CueID     string    `json:"cueId"`
	Success   bool      `json:"success"`
	PeakKg    int64     `json:"peakKg"`
	Deviation string    `json:"deviation"`
	Evidence  string    `json:"evidence"`
	Operator  string    `json:"operator,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type ValidationIssueState string

const (
	IssueNew        ValidationIssueState = "NEW"
	IssuePersisting ValidationIssueState = "PERSISTING"
	IssueResolved   ValidationIssueState = "RESOLVED"
)

type ValidationBatchIssue struct {
	Identity     string               `json:"identity"`
	RuleCode     string               `json:"ruleCode"`
	Severity     Severity             `json:"severity"`
	Message      string               `json:"message"`
	AffectedRefs []string             `json:"affectedRefs"`
	State        ValidationIssueState `json:"state"`
}

type ValidationBatch struct {
	ID               string                 `json:"id"`
	CaseID           string                 `json:"caseId"`
	AggregateVersion uint64                 `json:"aggregateVersion"`
	InputRevisions   map[string]uint32      `json:"inputRevisions"`
	StartedAt        time.Time              `json:"startedAt"`
	CompletedAt      time.Time              `json:"completedAt"`
	FinalStatus      RiggingStatus          `json:"finalStatus"`
	Stale            bool                   `json:"stale"`
	ChangedRefs      []string               `json:"changedRefs,omitempty"`
	Issues           []ValidationBatchIssue `json:"issues"`
}

type RemediationAttempt struct {
	ID                string            `json:"id"`
	CaseID            string            `json:"caseId"`
	FindingID         string            `json:"findingId"`
	ObservedRevisions map[string]uint32 `json:"observedRevisions"`
	Note              string            `json:"note"`
	SubmittedBy       string            `json:"submittedBy"`
	SubmittedAt       time.Time         `json:"submittedAt"`
	RecheckType       FindingSource     `json:"recheckType"`
	RecheckInput      *CueResult        `json:"recheckInput,omitempty"`
	Passed            bool              `json:"passed"`
	Conclusion        string            `json:"conclusion"`
}
type RehearsalRun struct {
	ID         string           `json:"id"`
	CaseID     string           `json:"caseId"`
	StartedAt  time.Time        `json:"startedAt"`
	FinishedAt *time.Time       `json:"finishedAt,omitempty"`
	CueResults []CueResult      `json:"cueResults"`
	Operator   string           `json:"operator"`
	Outcome    RehearsalOutcome `json:"outcome"`
}
type ReleaseCertificate struct {
	ID              string    `json:"id"`
	CaseID          string    `json:"caseId"`
	Serial          uint64    `json:"serial"`
	FrozenVersion   uint64    `json:"frozenVersion"`
	SnapshotDigest  string    `json:"snapshotDigest"`
	AuditHeadDigest string    `json:"auditHeadDigest"`
	Reviewer        string    `json:"reviewer"`
	IssuedAt        time.Time `json:"issuedAt"`
}
type AuditEvent struct {
	Sequence         uint64    `json:"sequence"`
	CaseID           string    `json:"caseId"`
	Kind             string    `json:"kind"`
	AggregateVersion uint64    `json:"aggregateVersion"`
	Payload          []byte    `json:"payload"`
	PreviousDigest   string    `json:"previousDigest"`
	Digest           string    `json:"digest"`
	OccurredAt       time.Time `json:"occurredAt"`
}

// MatchesCertificatePayload reports whether the supplied audit event payload
// decodes into a release certificate whose signing fields exactly match the
// provided certificate record.  A payload that fails to decode is treated as a
// mismatch.  Times are compared with Equal so that the RFC3339Nano
// round-trips used by the audit log and persistence layer remain stable.
func MatchesCertificatePayload(cert *ReleaseCertificate, payload []byte) bool {
	if cert == nil {
		return false
	}
	var decoded ReleaseCertificate
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return false
	}
	return decoded.ID == cert.ID &&
		decoded.CaseID == cert.CaseID &&
		decoded.Serial == cert.Serial &&
		decoded.FrozenVersion == cert.FrozenVersion &&
		decoded.SnapshotDigest == cert.SnapshotDigest &&
		decoded.AuditHeadDigest == cert.AuditHeadDigest &&
		decoded.Reviewer == cert.Reviewer &&
		decoded.IssuedAt.Equal(cert.IssuedAt)
}
