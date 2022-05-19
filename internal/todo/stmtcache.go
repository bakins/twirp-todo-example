package todo

import (
	"context"
	"database/sql"
	"sync"
)

type stmtCache struct {
	lock       sync.RWMutex
	statements map[string]*sql.Stmt
	preparer   preparerContext
}

type preparerContext interface {
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

func newStmtCache(preparer preparerContext) *stmtCache {
	c := stmtCache{
		statements: make(map[string]*sql.Stmt),
		preparer:   preparer,
	}

	return &c
}

func (c *stmtCache) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	for key, stmt := range c.statements {
		delete(c.statements, key)

		if stmt == nil {
			continue
		}

		_ = stmt.Close()
	}
}

func (c *stmtCache) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	c.lock.RLock()
	stmt, ok := c.statements[query]
	c.lock.RUnlock()

	if ok {
		return stmt, nil
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	stmt, err := c.preparer.PrepareContext(ctx, query)
	if err == nil {
		c.statements[query] = stmt
	}
	return stmt, err
}

func (c *stmtCache) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return stmt.QueryContext(ctx, args...)
}

func (c *stmtCache) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return stmt.ExecContext(ctx, args...)
}
