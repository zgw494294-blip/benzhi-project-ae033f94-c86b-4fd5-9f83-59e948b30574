package concurrentdigesterworkspace_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"stage-rigging-clearance/internal/audit"
	"testing"
)

type digestInput struct {
	previous string
	caseID   string
	version  uint64
	kind     string
	payload  []byte
}

func expectedEventDigest(in digestInput) string {
	canonical, _ := json.Marshal(struct {
		Previous string          `json:"previous"`
		CaseID   string          `json:"caseId"`
		Version  uint64          `json:"version"`
		Kind     string          `json:"kind"`
		Payload  json.RawMessage `json:"payload"`
	}{in.previous, in.caseID, in.version, in.kind, in.payload})
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func TestConcurrentDigestOperationsRemainIsolated(t *testing.T) {
	digester := audit.NewDigester()
	const workers = 12
	const rounds = 64
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	failures := make(chan string, workers)

	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			ready <- struct{}{}
			<-start
			for round := 0; round < rounds; round++ {
				if worker%3 == 0 {
					snapshot := []byte(fmt.Sprintf(`{"caseId":"case-%02d","version":%d,"padding":"%064d"}`, worker, round, worker*round+1))
					sum := sha256.Sum256(snapshot)
					if got, want := digester.SnapshotDigest(snapshot), hex.EncodeToString(sum[:]); got != want {
						failures <- fmt.Sprintf("snapshot digest crossed request boundary: got %s want %s", got, want)
						return
					}
					continue
				}
				in := digestInput{
					previous: fmt.Sprintf("head-%02d-%03d", worker, round),
					caseID:   fmt.Sprintf("case-%02d", worker),
					version:  uint64(round + 1),
					kind:     "CASE_VALIDATED",
					payload:  []byte(fmt.Sprintf(`{"worker":%d,"round":%d,"refs":["P-%02d"]}`, worker, round, worker)),
				}
				if got, want := digester.EventDigest(in.previous, in.caseID, in.version, in.kind, in.payload), expectedEventDigest(in); got != want {
					failures <- fmt.Sprintf("event digest crossed request boundary: got %s want %s", got, want)
					return
				}
			}
			failures <- ""
		}()
	}

	for worker := 0; worker < workers; worker++ {
		<-ready
	}
	close(start)
	for worker := 0; worker < workers; worker++ {
		if failure := <-failures; failure != "" {
			t.Error(failure)
		}
	}
}
