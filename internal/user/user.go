// Package user provides a phone-keyed user repository backed by raw SQL
// against MySQL, per CLAUDE.md's no-ORM rule.
package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup finds no matching user row.
var ErrNotFound = errors.New("user: not found")

// User mirrors a row in the users table.
type User struct {
	ID          uint64
	PhoneNumber string
	Email       sql.NullString
	CreatedAt   time.Time
}

// GetByPhone looks up a user by phone_number, wrapping sql.ErrNoRows into
// ErrNotFound.
func GetByPhone(ctx context.Context, db *sql.DB, phone string) (User, error) {
	var u User
	row := db.QueryRowContext(ctx,
		`SELECT id, phone_number, email, created_at FROM users WHERE phone_number = ?`,
		phone,
	)
	if err := row.Scan(&u.ID, &u.PhoneNumber, &u.Email, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, fmt.Errorf("user get by phone: %w", ErrNotFound)
		}
		return User{}, fmt.Errorf("user get by phone: %w", err)
	}
	return u, nil
}

// Create inserts a new user row for the given phone number.
func Create(ctx context.Context, db *sql.DB, phone string) (User, error) {
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (phone_number) VALUES (?)`,
		phone,
	); err != nil {
		return User{}, fmt.Errorf("user create: %w", err)
	}

	return GetByPhone(ctx, db, phone)
}
