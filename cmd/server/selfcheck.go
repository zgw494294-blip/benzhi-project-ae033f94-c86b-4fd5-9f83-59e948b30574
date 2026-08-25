package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"stage-rigging-clearance/internal/domain"
	"time"
)

type checkCase struct {
	ID          string                     `json:"id"`
	Version     uint64                     `json:"version"`
	Status      domain.RiggingStatus       `json:"status"`
	Rehearsals  []domain.RehearsalRun      `json:"rehearsals"`
	Certificate *domain.ReleaseCertificate `json:"certificate"`
}

func performSelfcheck(srv *http.Server, base string) error {
	client := &http.Client{Timeout: 4 * time.Second}
	var c checkCase
	step := func(method, path string, body any) error {
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(method, base+path, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, data)
		}
		if err := json.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("解析自检响应: %w", err)
		}
		return nil
	}
	future := time.Now().Add(2 * time.Hour).UTC()
	if err := step("POST", "/api/v1/rigging-cases", map[string]any{"showName": "自检演出", "venueLimitKg": 5000, "owner": "自检机械师", "scheduledAt": future, "idempotencyKey": "self-create"}); err != nil {
		return err
	}
	id := c.ID
	if err := step("PUT", "/api/v1/rigging-cases/"+id+"/load-points", map[string]any{"expectedVersion": c.Version, "idempotencyKey": "self-point", "id": "P-01", "equipmentCode": "HOIST-A", "ratedCapacityKg": 2000, "staticLoadKg": 500, "dynamicFactorPermille": 1250}); err != nil {
		return err
	}
	if err := step("PUT", "/api/v1/rigging-cases/"+id+"/cues", map[string]any{"expectedVersion": c.Version, "idempotencyKey": "self-cue", "id": "CUE-01", "sequence": 1, "action": "升起主景片", "equipmentCodes": []string{"HOIST-A"}, "expectedDurationMs": 5000}); err != nil {
		return err
	}
	if err := step("POST", "/api/v1/rigging-cases/"+id+"/validate", map[string]any{"expectedVersion": c.Version, "idempotencyKey": "self-validate"}); err != nil {
		return err
	}
	if c.Status != domain.StatusValidated {
		return fmt.Errorf("校验后状态异常: %s", c.Status)
	}
	if err := step("POST", "/api/v1/rigging-cases/"+id+"/rehearsals", map[string]any{"expectedVersion": c.Version, "idempotencyKey": "self-start", "operator": "自检操作员"}); err != nil {
		return err
	}
	runID := c.Rehearsals[len(c.Rehearsals)-1].ID
	if err := step("POST", "/api/v1/rigging-cases/"+id+"/rehearsals/complete", map[string]any{"expectedVersion": c.Version, "idempotencyKey": "self-complete", "runId": runID, "results": []map[string]any{{"cueId": "CUE-01", "success": true, "peakKg": 620, "evidence": "现场仪表读数稳定"}}}); err != nil {
		return err
	}
	if err := step("POST", "/api/v1/rigging-cases/"+id+"/release", map[string]any{"expectedVersion": c.Version, "idempotencyKey": "self-release", "reviewer": "自检复核员"}); err != nil {
		return err
	}
	if c.Status != domain.StatusReleased || c.Certificate == nil {
		return fmt.Errorf("未生成放行凭据")
	}
	var verify struct {
		Valid bool `json:"valid"`
	}
	resp, err := client.Get(base + "/api/v1/rigging-cases/" + id + "/certificate/verify")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&verify); err != nil || !verify.Valid {
		return fmt.Errorf("凭据摘要校验失败")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	fmt.Println("selfcheck passed: 已经由真实 HTTP 完成创建、编制、校验、排练、复核、放行与摘要校验")
	return nil
}
