package db

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestUp_Idempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set; skipping integration test")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := Up(db); err != nil {
		t.Fatalf("first Up() call failed: %v", err)
	}

	if err := Up(db); err != nil {
		t.Errorf("second Up() call must be idempotent, got error: %v", err)
	}
}
