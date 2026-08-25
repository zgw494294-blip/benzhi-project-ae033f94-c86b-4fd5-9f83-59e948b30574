package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestExtendedWorkflowThroughPublicHTTP(t *testing.T) {
	srv, repo := testServer(t)
	defer srv.Close()
	defer repo.Close()

	created := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases", `{"showName":"扩展流程演出","venueLimitKg":3000,"owner":"机械师","scheduledAt":"2026-08-25T10:00:00Z","idempotencyKey":"create-extended"}`)
	caseID := stringField(t, created, "id")
	changeBody := `{"expectedVersion":1,"idempotencyKey":"change-1","loadPoints":[{"id":"P-01","equipmentCode":"H-01","ratedCapacityKg":1000,"staticLoadKg":200,"dynamicFactorPermille":1250}],"cues":[{"id":"C-01","sequence":1,"action":"升起","equipmentCodes":["H-01"],"mutexGroup":"","expectedDurationMs":1000}]}`
	changed := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases/"+caseID+"/change-sets", changeBody)
	if numberField(t, changed, "version") != 2 {
		t.Fatalf("批量变更未只推进一次 version: %v", changed)
	}
	retried := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases/"+caseID+"/change-sets", changeBody)
	if numberField(t, retried, "version") != 2 {
		t.Fatalf("幂等重试推进了 version: %v", retried)
	}

	validated := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases/"+caseID+"/validate", `{"expectedVersion":2,"idempotencyKey":"validate-1"}`)
	if stringField(t, validated, "status") != "VALIDATED" {
		t.Fatalf("校验未通过: %v", validated)
	}
	batches, ok := validated["validationBatches"].([]any)
	if !ok || len(batches) != 1 {
		t.Fatalf("校验批次未返回: %v", validated)
	}

	started := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases/"+caseID+"/rehearsals", `{"expectedVersion":3,"idempotencyKey":"run-start","operator":"操作员"}`)
	runs := started["rehearsals"].([]any)
	runID := stringField(t, runs[0].(map[string]any), "id")
	requestJSON(t, srv, http.MethodPut, "/api/v1/rigging-cases/"+caseID+"/rehearsals/"+runID+"/cue-results/C-01", `{"expectedVersion":4,"idempotencyKey":"cue-save","success":true,"peakKg":250,"deviation":"","evidence":"仪表照片","operator":"操作员"}`)
	view := requestJSON(t, srv, http.MethodGet, "/api/v1/rigging-cases/"+caseID, "")
	progress := view["rehearsalProgress"].(map[string]any)
	if numberField(t, progress, "recordedCount") != 1 || len(progress["pendingCueIds"].([]any)) != 0 {
		t.Fatalf("断点进度错误: %v", progress)
	}
	completed := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases/"+caseID+"/rehearsals/complete", `{"expectedVersion":5,"idempotencyKey":"run-complete","runId":"`+runID+`","results":[]}`)
	if stringField(t, completed, "status") != "READY_FOR_REVIEW" {
		t.Fatalf("排练完成状态错误: %v", completed)
	}
	released := requestJSON(t, srv, http.MethodPost, "/api/v1/rigging-cases/"+caseID+"/release", `{"expectedVersion":6,"idempotencyKey":"release-1","reviewer":"复核员"}`)
	cert := released["certificate"].(map[string]any)
	serial := int(numberField(t, cert, "serial"))
	report := requestJSON(t, srv, http.MethodGet, "/api/v1/release-certificates?serial="+strconv.Itoa(serial), "")
	if valid, _ := report["valid"].(bool); !valid || len(report["checks"].([]any)) != 5 {
		t.Fatalf("凭据分项报告异常: %v", report)
	}
}

func requestJSON(t *testing.T, srv *httptest.Server, method, path, body string) map[string]any {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s 返回 %d: %v", method, path, resp.StatusCode, result)
	}
	return result
}

func stringField(t *testing.T, object map[string]any, field string) string {
	t.Helper()
	value, ok := object[field].(string)
	if !ok {
		t.Fatalf("字段 %s 不是字符串: %v", field, object[field])
	}
	return value
}
func numberField(t *testing.T, object map[string]any, field string) float64 {
	t.Helper()
	value, ok := object[field].(float64)
	if !ok {
		t.Fatalf("字段 %s 不是数字: %v", field, object[field])
	}
	return value
}
