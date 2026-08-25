package store

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"sync"
)

type Store struct{ db *sql.DB }

var databaseHandles sync.Map

func openDatabase(path, dsn string) (*sql.DB, error) {
	if path != ":memory:" {
		if cached, ok := databaseHandles.Load(path); ok {
			return cached.(*sql.DB), nil
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		return db, nil
	}
	actual, loaded := databaseHandles.LoadOrStore(path, db)
	if loaded {
		_ = db.Close()
		return actual.(*sql.DB), nil
	}
	return db, nil
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file:rigging?mode=memory&cache=shared"
	}
	db, err := openDatabase(path, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) migrate(ctx context.Context) error {
	for _, statement := range schemaStatements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("数据库迁移失败: %w", err)
		}
	}
	return nil
}
