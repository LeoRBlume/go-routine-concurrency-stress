package models

import "go-routine-stress/internal/services"

// CombinedResponse is returned by all endpoints on success.
type CombinedResponse struct {
	ServiceAData services.ServiceAData `json:"serviceAData"`
	ServiceBData services.ServiceBData `json:"serviceBData"`
	Mode         string                `json:"mode"`
	TotalMs      int64                 `json:"totalMs"`
}

// ErrorResponse is returned by all endpoints on failure.
type ErrorResponse struct {
	Mode    string `json:"mode"`
	TotalMs int64  `json:"totalMs"`
	Error   string `json:"error"`
}
