package domain

import (
	"sort"
	"strings"
	"time"
)

type RiggingCase struct {
	ID                  string               `json:"id"`
	ShowName            string               `json:"showName"`
	VenueLimitKg        int64                `json:"venueLimitKg"`
	Owner               string               `json:"owner"`
	ScheduledAt         time.Time            `json:"scheduledAt"`
	Status              RiggingStatus        `json:"status"`
	Version             uint64               `json:"version"`
	CreatedAt           time.Time            `json:"createdAt"`
	ReleasedAt          *time.Time           `json:"releasedAt,omitempty"`
	LoadPoints          []LoadPoint          `json:"loadPoints"`
	Cues                []SceneCue           `json:"cues"`
	Findings            []SafetyFinding      `json:"findings"`
	Rehearsals          []RehearsalRun       `json:"rehearsals"`
	ValidationBatches   []ValidationBatch    `json:"validationBatches"`
	RemediationAttempts []RemediationAttempt `json:"remediationAttempts"`
	Certificate         *ReleaseCertificate  `json:"certificate,omitempty"`
}

func NewCase(id, showName string, venueLimitKg int64, owner string, scheduled, now time.Time) (*RiggingCase, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(showName) == "" || strings.TrimSpace(owner) == "" {
		return nil, invalid("INVALID_CASE", "编号、演出名称和负责人不能为空")
	}
	if venueLimitKg <= 0 {
		return nil, invalid("INVALID_LIMIT", "场地额定限制必须大于零")
	}
	if scheduled.Before(now) {
		return nil, invalid("INVALID_SCHEDULE", "计划开演时间不能早于创建时间")
	}
	return &RiggingCase{ID: id, ShowName: strings.TrimSpace(showName), VenueLimitKg: venueLimitKg, Owner: strings.TrimSpace(owner), ScheduledAt: scheduled.UTC(), CreatedAt: now.UTC(), Status: StatusDraft, Version: 1, LoadPoints: []LoadPoint{}, Cues: []SceneCue{}, Findings: []SafetyFinding{}, Rehearsals: []RehearsalRun{}, ValidationBatches: []ValidationBatch{}, RemediationAttempts: []RemediationAttempt{}}, nil
}

func (c *RiggingCase) markValidationStale(refs ...string) {
	if len(c.ValidationBatches) == 0 {
		return
	}
	b := &c.ValidationBatches[len(c.ValidationBatches)-1]
	if b.Stale {
		b.ChangedRefs = normalizedRefs(append(b.ChangedRefs, refs...))
		return
	}
	b.Stale = true
	b.ChangedRefs = normalizedRefs(refs)
}
func (c *RiggingCase) EnsureMutable() error {
	if c.Status == StatusReleased {
		return invalid("CASE_FROZEN", "已放行方案不可修改")
	}
	return nil
}
func (c *RiggingCase) RequireStatus(states ...RiggingStatus) error {
	for _, s := range states {
		if c.Status == s {
			return nil
		}
	}
	return invalid("INVALID_STATE", "状态 %s 不允许该操作", c.Status)
}
func (c *RiggingCase) Bump() { c.Version++ }
func (c *RiggingCase) OpenBlockingFindings() []SafetyFinding {
	var out []SafetyFinding
	for _, f := range c.Findings {
		if f.Status == FindingOpen && f.Severity == SeverityBlocking {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (c *RiggingCase) CueByID(id string) *SceneCue {
	for i := range c.Cues {
		if c.Cues[i].ID == id {
			return &c.Cues[i]
		}
	}
	return nil
}
func (c *RiggingCase) PointByID(id string) *LoadPoint {
	for i := range c.LoadPoints {
		if c.LoadPoints[i].ID == id {
			return &c.LoadPoints[i]
		}
	}
	return nil
}

func (c *RiggingCase) RevisionOf(ref string) uint32 {
	if point := c.PointByID(ref); point != nil {
		return point.Revision
	}
	if cue := c.CueByID(ref); cue != nil {
		return cue.Revision
	}
	return 0
}
