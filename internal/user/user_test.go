package user

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/g4uk/kai/internal/db"
)

// TestGetByPhone_NotFound, TestCreateThenGetByPhone_RoundTrip and
// TestCreate_DuplicatePhone below drive internal/user/user.go, which does not
// exist yet (specs/user-auth/plan.md step 2). Expect a compile error until
// user.go defines: type User struct{ ID uint64; PhoneNumber string; Email
// sql.NullString; CreatedAt time.Time }, var ErrNotFound error,
// GetByPhone(ctx, db, phone) (User, error), Create(ctx, db, phone) (User, error).
//
// Gated on TEST_DSN (same convention as internal/db tests). Calls db.Up to
// ensure the schema (including the phone_number column from step 1) exists
// before exercising the repository, per plan.md step 2.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set; skipping integration test")
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := db.Up(sqlDB); err != nil {
		t.Fatalf("db.Up: %v", err)
	}

	return sqlDB
}

func TestGetByPhone_NotFound(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()

	_, err := GetByPhone(ctx, sqlDB, "+15559990001")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByPhone unknown phone: got err %v, want ErrNotFound", err)
	}
}

func TestCreateThenGetByPhone_RoundTrip(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	phone := "+15559990002"

	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE phone_number = ?`, phone)
	})

	created, err := Create(ctx, sqlDB, phone)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.PhoneNumber != phone {
		t.Errorf("created.PhoneNumber = %q, want %q", created.PhoneNumber, phone)
	}
	if created.ID == 0 {
		t.Error("created.ID = 0, want non-zero auto-increment ID")
	}

	got, err := GetByPhone(ctx, sqlDB, phone)
	if err != nil {
		t.Fatalf("GetByPhone after Create: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetByPhone.ID = %d, want %d (from Create)", got.ID, created.ID)
	}
	if got.PhoneNumber != phone {
		t.Errorf("GetByPhone.PhoneNumber = %q, want %q", got.PhoneNumber, phone)
	}
}

func TestCreate_DuplicatePhone(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	phone := "+15559990003"

	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE phone_number = ?`, phone)
	})

	if _, err := Create(ctx, sqlDB, phone); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	if _, err := Create(ctx, sqlDB, phone); err == nil {
		t.Fatal("second Create with same phone_number: want wrapped unique-constraint error, got nil")
	}
}
