package corrupt_time_propagation_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
	"stage-rigging-clearance/internal/store"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCorruptPersistedTimeIsReported(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "corrupt-time.db")
	repo, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	c, err := domain.NewCase("case-corrupt-time", "损坏检测演出", 5000, "机械师", now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WithinTransaction(ctx, func(tx application.Transaction) error {
		return tx.CreateCase(ctx, c)
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE rigging_cases SET scheduled_at='not-a-timestamp' WHERE id=?`, c.ID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err = store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	loaded, err := repo.ViewCase(ctx, c.ID)
	if err == nil {
		t.Fatalf("损坏的持久化时间被静默转换: scheduledAt=%s", loaded.ScheduledAt.Format(time.RFC3339Nano))
	}
}
