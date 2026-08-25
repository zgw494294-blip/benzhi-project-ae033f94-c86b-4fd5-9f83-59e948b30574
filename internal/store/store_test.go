package store_test

import (
	"context"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/domain"
	"stage-rigging-clearance/internal/store"
	"testing"
	"time"
)

func TestAggregatePersistsAndVersionConflicts(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	now := time.Now().UTC()
	c, err := domain.NewCase("case-store", "持久化演出", 4000, "负责人", now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WithinTransaction(ctx, func(tx application.Transaction) error { return tx.CreateCase(ctx, c) }); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.ViewCase(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 || loaded.ShowName != c.ShowName {
		t.Fatalf("恢复结果异常: %+v", loaded)
	}
	loaded.Bump()
	if err := repo.WithinTransaction(ctx, func(tx application.Transaction) error { return tx.SaveCase(ctx, loaded, 99) }); err == nil {
		t.Fatal("错误 expectedVersion 未产生冲突")
	}
}

func TestReleasedRowsAreImmutable(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	now := time.Now().UTC()
	c, _ := domain.NewCase("case-release", "放行演出", 4000, "负责人", now.Add(time.Hour), now)
	if err := repo.WithinTransaction(ctx, func(tx application.Transaction) error { return tx.CreateCase(ctx, c) }); err != nil {
		t.Fatal(err)
	}
	c.Status = domain.StatusReleased
	c.Version = 2
	c.ReleasedAt = &now
	if err := repo.WithinTransaction(ctx, func(tx application.Transaction) error { return tx.SaveCase(ctx, c, 1) }); err != nil {
		t.Fatal(err)
	}
	c.Owner = "篡改者"
	c.Version = 3
	if err := repo.WithinTransaction(ctx, func(tx application.Transaction) error { return tx.SaveCase(ctx, c, 2) }); err == nil {
		t.Fatal("RELEASED 数据库记录仍可更新")
	}
}
