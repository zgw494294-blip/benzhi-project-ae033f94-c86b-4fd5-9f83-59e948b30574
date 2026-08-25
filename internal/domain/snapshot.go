package domain

import (
	"encoding/json"
	"sort"
)

type FrozenSnapshot struct {
	CaseID              string               `json:"caseId"`
	ShowName            string               `json:"showName"`
	VenueLimitKg        int64                `json:"venueLimitKg"`
	Owner               string               `json:"owner"`
	ScheduledAt         string               `json:"scheduledAt"`
	Status              RiggingStatus        `json:"status"`
	Version             uint64               `json:"version"`
	LoadPoints          []LoadPoint          `json:"loadPoints"`
	Cues                []SceneCue           `json:"cues"`
	Findings            []SafetyFinding      `json:"findings"`
	Rehearsals          []RehearsalRun       `json:"rehearsals"`
	ValidationBatches   []ValidationBatch    `json:"validationBatches"`
	RemediationAttempts []RemediationAttempt `json:"remediationAttempts"`
}

func (c *RiggingCase) FreezeSnapshot() ([]byte, error) {
	points := append([]LoadPoint(nil), c.LoadPoints...)
	cues := append([]SceneCue(nil), c.Cues...)
	findings := append([]SafetyFinding(nil), c.Findings...)
	runs := append([]RehearsalRun(nil), c.Rehearsals...)
	batches := append([]ValidationBatch(nil), c.ValidationBatches...)
	attempts := append([]RemediationAttempt(nil), c.RemediationAttempts...)
	sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
	sort.Slice(cues, func(i, j int) bool {
		if cues[i].Sequence == cues[j].Sequence {
			return cues[i].ID < cues[j].ID
		}
		return cues[i].Sequence < cues[j].Sequence
	})
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	sort.Slice(batches, func(i, j int) bool { return batches[i].ID < batches[j].ID })
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].ID < attempts[j].ID })
	snapshot := FrozenSnapshot{
		CaseID: c.ID, ShowName: c.ShowName, VenueLimitKg: c.VenueLimitKg, Owner: c.Owner,
		ScheduledAt: c.ScheduledAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		Status:      c.Status, Version: c.Version, LoadPoints: points, Cues: cues,
		Findings: findings, Rehearsals: runs, ValidationBatches: batches, RemediationAttempts: attempts,
	}
	return json.Marshal(snapshot)
}
