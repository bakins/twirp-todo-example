package database

import (
	"context"
	"database/sql"

	"github.com/XSAM/otelsql"
	migrate "github.com/golang-migrate/migrate/v4"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	// sqlite datbase driver
	_ "github.com/mattn/go-sqlite3"

	// migrate file source
	_ "github.com/golang-migrate/migrate/v4/source/file"

	// migrate database support
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
)

type Config struct {
	Filename        string `kong:"required,default=./data/data.db"`
	SchemaDirectory string `kong:",default=./schema"`
}

func (c Config) Build(ctx context.Context) (*sql.DB, error) {
	dsn := c.Filename + "?_journal_mode=WAL&cache=shared"

	if c.SchemaDirectory != "" {
		m, err := migrate.New("file://"+c.SchemaDirectory, "sqlite3://"+dsn)
		if err != nil {
			return nil, err
		}

		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return nil, err
		}

	}

	return otelsql.Open("sqlite3", "file:"+dsn, otelsql.WithAttributes(
		semconv.DBSystemSqlite,
	))
}
