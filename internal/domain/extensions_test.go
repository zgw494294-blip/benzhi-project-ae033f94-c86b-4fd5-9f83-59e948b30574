package domain

import (
	"errors"
	"testing"
	"time"
)

func TestChangeSetPreflightIsAtomicAndReportsAllItems(t *testing.T) {
	c := validCase(t)
	beforeVersion := c.Version
	parent := "missing"
	issues := c.ApplyChangeSet(ChangeSet{
		LoadPoints: []LoadPoint{
			{ID: "P-02", EquipmentCode: "H-01", ParentPointID: &parent, RatedCapacityKg: 1000, StaticLoadKg: 100, DynamicFactorPermille: 1250},
		},
		Cues: []SceneCue{
			{ID: "C-02", Sequence: 1, Action: "降下", EquipmentCodes: []string{"H-01"}, ExpectedDurationMs: 1000},
		},
	})
	got := map[string]bool{}
	for _, issue := range issues {
		got[issue.Code] = true
	}
	for _, code := range []string{"DUPLICATE_EQUIPMENT", "UNKNOWN_PARENT", "DUPLICATE_SEQUENCE"} {
		if !got[code] {
			t.Fatalf("缺少错误 %s: %+v", code, issues)
		}
	}
	if c.Version != beforeVersion || c.PointByID("P-02") != nil || c.CueByID("C-02") != nil {
		t.Fatal("预检失败后聚合发生了部分变更")
	}
}

func TestCascadeLoadUsesRootsForVenueAndTracksContributors(t *testing.T) {
	now := time.Now().UTC()
	c, _ := NewCase("tree", "树载荷", 1400, "机械师", now.Add(time.Hour), now)
	p1, p2 := "P-ROOT-1", "P-ROOT-2"
	c.LoadPoints = []LoadPoint{
		{ID: p1, CaseID: c.ID, EquipmentCode: "H1", RatedCapacityKg: 1000, StaticLoadKg: 200, DynamicFactorPermille: 1000, Revision: 1},
		{ID: "P-C1", CaseID: c.ID, EquipmentCode: "H2", ParentPointID: &p1, RatedCapacityKg: 500, StaticLoadKg: 450, DynamicFactorPermille: 1000, Revision: 1},
		{ID: "P-C2", CaseID: c.ID, EquipmentCode: "H3", ParentPointID: &p1, RatedCapacityKg: 500, StaticLoadKg: 450, DynamicFactorPermille: 1000, Revision: 1},
		{ID: p2, CaseID: c.ID, EquipmentCode: "H4", RatedCapacityKg: 800, StaticLoadKg: 200, DynamicFactorPermille: 1000, Revision: 1},
	}
	c.Cues = []SceneCue{{ID: "C", CaseID: c.ID, Sequence: 1, Action: "动作", EquipmentCodes: []string{"H1"}, ExpectedDurationMs: 1000, Revision: 1}}
	results := Evaluate(c)
	foundCapacity, foundVenue := false, false
	for _, result := range results {
		if result.Code == "POINT_CAPACITY" && len(result.Refs) == 3 {
			foundCapacity = true
		}
		if result.Code == "VENUE_CAPACITY" {
			foundVenue = true
		}
	}
	if !foundCapacity || foundVenue {
		t.Fatalf("级联或场地净载结果异常: %+v", results)
	}
}

func TestRehearsalCueResultsCanResumeAndCompletionListsMissing(t *testing.T) {
	c := validCase(t)
	now := time.Now().UTC()
	c.RunValidation("batch", now, now, func() string { return "finding" })
	run, err := c.StartRehearsal("run", "操作员", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SaveCueResult(run.ID, CueResult{CueID: "C-01", Success: true, PeakKg: 600, Evidence: "仪表照片"}, "操作员", now); err != nil {
		t.Fatal(err)
	}
	if err := c.CompleteRehearsal(run.ID, nil, now, func() string { return "finding" }); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusReadyForReview || len(c.Rehearsals[0].CueResults) != 1 {
		t.Fatalf("逐提示保存结果未用于完成: %+v", c.Rehearsals[0])
	}

	c2 := validCase(t)
	c2.Cues = append(c2.Cues, SceneCue{ID: "C-02", CaseID: c2.ID, Sequence: 2, Action: "降下", EquipmentCodes: []string{"H-01"}, ExpectedDurationMs: 1000, Revision: 1})
	c2.RunValidation("batch-2", now, now, func() string { return "finding-2" })
	run2, _ := c2.StartRehearsal("run-2", "操作员", now)
	_ = c2.SaveCueResult(run2.ID, CueResult{CueID: "C-01", Success: true}, "操作员", now)
	err = c2.CompleteRehearsal(run2.ID, nil, now, func() string { return "unused" })
	var violation Violation
	if !errors.As(err, &violation) || violation.Code != "INCOMPLETE_REHEARSAL" || c2.Rehearsals[0].Outcome != OutcomePending {
		t.Fatalf("缺失提示未返回结构化错误: %v", err)
	}
}
