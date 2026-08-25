package domain

type ReviewCheck struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Passed  bool   `json:"passed"`
	Details string `json:"details"`
}

func (c *RiggingCase) ReviewChecklist() []ReviewCheck {
	checks := []ReviewCheck{
		{Code: "STATUS_READY", Label: "方案处于待复核状态", Passed: c.Status == StatusReadyForReview, Details: string(c.Status)},
		{Code: "LOADS_PRESENT", Label: "吊点载荷清单完整", Passed: len(c.LoadPoints) > 0, Details: countText(len(c.LoadPoints), "个吊点")},
		{Code: "CUES_PRESENT", Label: "换景提示序列完整", Passed: len(c.Cues) > 0, Details: countText(len(c.Cues), "条提示")},
		{Code: "REHEARSAL_FINISHED", Label: "现场排练已经完成", Passed: hasFinishedRehearsal(c.Rehearsals), Details: countText(len(c.Rehearsals), "次排练")},
		{Code: "NO_OPEN_BLOCKERS", Label: "全部阻断问题已关闭", Passed: len(c.OpenBlockingFindings()) == 0, Details: countText(len(c.OpenBlockingFindings()), "个未关闭阻断问题")},
		{Code: "NOT_PREVIOUSLY_RELEASED", Label: "尚未签发放行凭据", Passed: c.Certificate == nil && c.ReleasedAt == nil, Details: "凭据只允许签发一次"},
	}
	return checks
}

func (c *RiggingCase) EnsureReadyForRelease() error {
	for _, check := range c.ReviewChecklist() {
		if !check.Passed {
			return invalid("REVIEW_INCOMPLETE", "%s：%s", check.Label, check.Details)
		}
	}
	return nil
}

func hasFinishedRehearsal(runs []RehearsalRun) bool {
	for _, run := range runs {
		if run.FinishedAt != nil && run.Outcome != OutcomePending && len(run.CueResults) > 0 {
			return true
		}
	}
	return false
}

func countText(value int, suffix string) string {
	if value == 0 {
		return "0" + suffix
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	n := value
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:]) + suffix
}
