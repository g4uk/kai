package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
	Error  string `json:"error,omitempty"`
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status: "ok",
		MySQL:  "ok",
		Redis:  "ok",
	}

	var errs []string
	if err := h.DB.Ping(r.Context()); err != nil {
		resp.Status = "error"
		resp.MySQL = "error"
		errs = append(errs, "mysql: "+err.Error())
	}
	if err := h.Redis.Ping(r.Context()); err != nil {
		resp.Status = "error"
		resp.Redis = "error"
		errs = append(errs, "redis: "+err.Error())
	}
	if len(errs) > 0 {
		resp.Error = strings.Join(errs, "; ")
	}

	status := http.StatusOK
	if resp.Status == "error" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}
