package domain

import (
	"testing"
	"time"
)

func validCase(t *testing.T) *RiggingCase {
	t.Helper()
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	c, err := NewCase("case-1", "测试演出", 5000, "机械师", now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertLoadPoint(LoadPoint{ID: "P-01", EquipmentCode: "H-01", RatedCapacityKg: 2000, StaticLoadKg: 500, DynamicFactorPermille: 1250}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertCue(SceneCue{ID: "C-01", Sequence: 1, Action: "升起", EquipmentCodes: []string{"H-01"}, ExpectedDurationMs: 4000}); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestHappyPathReleaseAndFreeze(t *testing.T) {
	c := validCase(t)
	next := 0
	c.Validate(func() string { next++; return "finding" })
	if c.Status != StatusValidated {
		t.Fatalf("status=%s findings=%v", c.Status, c.Findings)
	}
	run, err := c.StartRehearsal("run-1", "操作员", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CompleteRehearsal(run.ID, []CueResult{{CueID: "C-01", Success: true, PeakKg: 630, Evidence: "仪表照片"}}, time.Now(), func() string { return "finding-2" }); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusReadyForReview {
		t.Fatalf("status=%s", c.Status)
	}
	if err := c.Release("复核员", time.Now()); err != nil {
		t.Fatal(err)
	}
	first, err := c.FreezeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := c.FreezeSnapshot()
	if string(first) != string(second) {
		t.Fatal("规范快照不稳定")
	}
	if err := c.UpsertCue(SceneCue{}); err == nil {
		t.Fatal("已放行方案仍可修改")
	}
}

func TestCapacityFindingIsDeterministic(t *testing.T) {
	c := validCase(t)
	c.LoadPoints[0].StaticLoadKg = 1900
	a, b := Evaluate(c), Evaluate(c)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatal("容量超限未生成稳定问题")
	}
	for i := range a {
		if a[i].Code != b[i].Code || a[i].Message != b[i].Message {
			t.Fatal("规则结果不确定")
		}
	}
	c.Validate(func() string { return "finding-capacity" })
	if c.Status == StatusValidated {
		t.Fatal("阻断问题不应进入 VALIDATED")
	}
}

func TestRehearsalDeviationRequiresTraceableRevision(t *testing.T) {
	c := validCase(t)
	c.Validate(func() string { return "unused" })
	run, _ := c.StartRehearsal("run", "操作员", time.Now())
	if err := c.CompleteRehearsal(run.ID, []CueResult{{CueID: "C-01", Success: false, PeakKg: 900, Deviation: "运行抖动", Evidence: "视频"}}, time.Now(), func() string { return "deviation" }); err != nil {
		t.Fatal(err)
	}
	if err := c.Remediate("deviation", 1, "仅写说明", time.Now()); err == nil {
		t.Fatal("无输入修订的整改不应通过")
	}
	if err := c.UpsertCue(SceneCue{ID: "C-01", Sequence: 1, Action: "降低速度后升起", EquipmentCodes: []string{"H-01"}, ExpectedDurationMs: 6000}); err != nil {
		t.Fatal(err)
	}
	if err := c.Remediate("deviation", 2, "降低速度并复核动作参数", time.Now()); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusReadyForReview {
		t.Fatalf("status=%s", c.Status)
	}
}
