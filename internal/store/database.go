package store

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"sync"
)

type Store struct{ handle *databaseHandle }

type databaseHandle struct {
	db     *sql.DB
	owner  bool // true 表示内存库独占，由该 Store 独自关闭
	refs   int64
	mu     sync.Mutex
}

var databaseHandles sync.Map

func openDatabase(path, dsn string) (*databaseHandle, error) {
	if path != ":memory:" {
		if cached, ok := databaseHandles.Load(path); ok {
			handle := cached.(*databaseHandle)
			handle.mu.Lock()
			handle.refs++
			handle.mu.Unlock()
			return handle, nil
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		return &databaseHandle{db: db, owner: true, refs: 1}, nil
	}
	handle := &databaseHandle{db: db, owner: false, refs: 1}
	actual, loaded := databaseHandles.LoadOrStore(path, handle)
	if loaded {
		_ = db.Close()
		existing := actual.(*databaseHandle)
		existing.mu.Lock()
		existing.refs++
		existing.mu.Unlock()
		return existing, nil
	}
	return handle, nil
}

func (h *databaseHandle) release() (closed bool) {
	h.mu.Lock()
	h.refs--
	if h.refs > 0 {
		h.mu.Unlock()
		return false
	}
	h.mu.Unlock()
	if h.owner {
		// 内存库独占，仅关闭句柄，不触碰全局缓存
		_ = h.db.Close()
		return true
	}
	// 文件库共享句柄：从缓存移除并关闭底层连接
	databaseHandles.Range(func(key, value any) bool {
		if value == h {
			databaseHandles.Delete(key)
		}
		return true
	})
	_ = h.db.Close()
	return true
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file:rigging?mode=memory&cache=shared"
	}
	handle, err := openDatabase(path, dsn)
	if err != nil {
		return nil, err
	}
	handle.db.SetMaxOpenConns(1)
	s := &Store{handle: handle}
	if err := s.migrate(ctx); err != nil {
		handle.release()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	if s.handle == nil {
		return nil
	}
	s.handle.release()
	return nil
}
func (s *Store) Ping(ctx context.Context) error { return s.handle.db.PingContext(ctx) }
func (s *Store) migrate(ctx context.Context) error {
	for _, statement := range schemaStatements {
		if _, err := s.handle.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("数据库迁移失败: %w", err)
		}
	}
	return nil
}
