package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/g4uk/kai/internal/db"
	"github.com/g4uk/kai/internal/handler"
	"github.com/g4uk/kai/internal/redisconn"
)

type dbPinger struct{ db *sql.DB }

func (p *dbPinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

type redisPinger struct{ c *redis.Client }

func (p *redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

func buildServer(database handler.Pinger, redis handler.Pinger) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", &handler.HealthHandler{DB: database, Redis: redis})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return mux
}

func main() {
	dsn := os.Getenv("DB_DSN")
	redisAddr := os.Getenv("REDIS_ADDR")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), db.MaxRetryBudget)
	defer dbCancel()
	sqlDB, err := db.Connect(dbCtx, dsn)
	if err != nil {
		slog.Error("db connect failed", "err", err)
		os.Exit(1)
	}

	if err := db.Up(sqlDB); err != nil {
		slog.Error("db migrate failed", "err", err)
		os.Exit(1)
	}

	redisCtx, redisCancel := context.WithTimeout(context.Background(), redisconn.MaxRetryBudget)
	defer redisCancel()
	redisClient, err := redisconn.Connect(redisCtx, redisAddr)
	if err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}

	mux := buildServer(&dbPinger{db: sqlDB}, &redisPinger{c: redisClient})

	slog.Info("starting api", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
