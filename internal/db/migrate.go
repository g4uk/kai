package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Up(sqlDB *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectMySQL, sqlDB, migrationFS)
	if err != nil {
		return fmt.Errorf("goose new provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
