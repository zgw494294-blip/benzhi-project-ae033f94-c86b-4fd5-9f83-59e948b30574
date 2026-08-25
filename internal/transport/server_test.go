package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/audit"
	"stage-rigging-clearance/internal/store"
	"stage-rigging-clearance/internal/transport"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC) }

type ids struct{ n atomic.Uint64 }

func (i *ids) NewID(prefix string) string { return prefix + "-test-" + string(rune('0'+i.n.Add(1))) }
func testServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	repo, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := application.NewService(repo, clock{}, &ids{}, audit.NewDigester())
	srv := httptest.NewServer(transport.NewServer(app).Handler())
	return srv, repo
}
func TestWorkbenchAndStructuredConflict(t *testing.T) {
	srv, repo := testServer(t)
	defer srv.Close()
	defer repo.Close()
	resp, err := http.Get(srv.URL + "/workbench")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("工作台不可访问: %d", resp.StatusCode)
	}
	resp.Body.Close()
	body := `{"showName":"接口演出","venueLimitKg":3000,"owner":"负责人","scheduledAt":"2026-08-25T10:00:00Z","idempotencyKey":"same"}`
	resp, err = http.Post(srv.URL+"/api/v1/rigging-cases", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("创建失败: %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	changed := strings.Replace(body, "接口演出", "另一演出", 1)
	resp, err = http.Post(srv.URL+"/api/v1/rigging-cases", "application/json", strings.NewReader(changed))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("幂等冲突状态=%d", resp.StatusCode)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.NewDecoder(resp.Body).Decode(&envelope) != nil || envelope.Error.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("非结构化冲突: %+v", envelope)
	}
	viewResp, err := http.Get(srv.URL + "/api/v1/rigging-cases/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer viewResp.Body.Close()
	var view struct {
		Timeline []struct {
			Kind string `json:"kind"`
		} `json:"timeline"`
	}
	if err := json.NewDecoder(viewResp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if len(view.Timeline) != 2 || view.Timeline[1].Kind != "WRITE_CONFLICT" {
		t.Fatalf("冲突未进入审计链: %+v", view.Timeline)
	}
}
