package handler

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
)

type DashboardHandler struct {
	DB               *sql.DB
	DingToken        string
	DingSecret       string
	DingClientID     string
	DingClientSecret string
	DingCallbackHost string
	HesiAppKey       string
	HesiSecret       string
	WebhookSecret    string
}

// round2 浮点数累加防精度尾巴(0.090000000001 → 0.09)
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

type CpsDaily struct {
	Date          string  `json:"date"`
	PayAmount     float64 `json:"payAmount"`
	PayCommission float64 `json:"payCommission"`
	PayUsers      int     `json:"payUsers"`
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": data,
	})
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": code,
		"msg":  msg,
	})
}
