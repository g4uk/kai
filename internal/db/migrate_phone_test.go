package db

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// TestUp_PhoneAuthMigration exercises the additive migration from
// specs/user-auth/plan.md step 1 (internal/db/migrations/002_add_phone_auth.sql):
// users.phone_number becomes a unique, nullable column and users.email becomes
// nullable. Gated on TEST_DSN, matching the convention in migrate_test.go.
func TestUp_PhoneAuthMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set; skipping integration test")
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	// t.Cleanup runs LIFO: registering Close here, before the row-cleanup
	// below, means Close runs LAST (after the DELETE cleanup has already run
	// against a still-open connection). A bare `defer sqlDB.Close()` would
	// instead run immediately when this function returns — before any
	// t.Cleanup callback — causing the DELETE below to silently no-op against
	// a closed connection and leak rows across test runs.
	t.Cleanup(func() { sqlDB.Close() })

	if err := Up(sqlDB); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE phone_number IN (?, ?)`, "+15551230001", "+15551230002")
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE email = ?`, "phone-migration-legacy@example.com")
	})

	t.Run("insert row with phone_number set and email NULL", func(t *testing.T) {
		_, err := sqlDB.Exec(
			`INSERT INTO users (phone_number, email) VALUES (?, NULL)`,
			"+15551230001",
		)
		if err != nil {
			t.Fatalf("insert phone-only row: %v", err)
		}
	})

	t.Run("insert row with old shape (email set, phone_number NULL)", func(t *testing.T) {
		_, err := sqlDB.Exec(
			`INSERT INTO users (email, phone_number) VALUES (?, NULL)`,
			"phone-migration-legacy@example.com",
		)
		if err != nil {
			t.Fatalf("insert email-only row: %v", err)
		}
	})

	t.Run("duplicate phone_number insert fails on unique constraint", func(t *testing.T) {
		_, err := sqlDB.Exec(
			`INSERT INTO users (phone_number, email) VALUES (?, NULL)`,
			"+15551230002",
		)
		if err != nil {
			t.Fatalf("insert first row with phone_number: %v", err)
		}

		_, err = sqlDB.Exec(
			`INSERT INTO users (phone_number, email) VALUES (?, NULL)`,
			"+15551230002",
		)
		if err == nil {
			t.Fatal("expected duplicate phone_number insert to fail on unique constraint, got nil error")
		}
	})
}
