package shareddbcloseinvalidation_test

import (
	"context"
	"path/filepath"
	"stage-rigging-clearance/internal/store"
	"testing"
)

func TestClosingOneStoreDoesNotInvalidatePeer(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rigging.db")
	first, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	second, err := store.Open(ctx, path)
	if err != nil {
		_ = first.Close()
		t.Fatalf("open second store: %v", err)
	}
	defer second.Close()

	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	if err := second.Ping(ctx); err != nil {
		t.Fatalf("second store became unusable after peer close: %v", err)
	}
}
