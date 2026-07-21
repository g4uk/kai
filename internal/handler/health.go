package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	DB    Pinger
	Redis Pinger
}

type healthResponse struct {
	Status string `json:"status"`
	MySQL  string `json:"mysql"`
	Redis  string `json:"redis"`
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status: "ok",
		MySQL:  "ok",
		Redis:  "ok",
	}

	if err := h.DB.Ping(r.Context()); err != nil {
		resp.Status = "error"
		resp.MySQL = "error"
	}

	if err := h.Redis.Ping(r.Context()); err != nil {
		resp.Status = "error"
		resp.Redis = "error"
	}

	status := http.StatusOK
	if resp.Status == "error" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
