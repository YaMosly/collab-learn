package models

import (
	"time"
)

type Board struct {
	ID        string    `json:"id"`
	CodeHTML  string    `json:"code_html"`
	CodeCSS   string    `json:"code_css"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Session struct {
	ID          string    `json:"id"`
	BoardID     string    `json:"board_id"`
	UserToken   string    `json:"user_token"`
	ConnectedAt time.Time `json:"connected_at"`
}

type BoardUpdate struct {
	BoardID  string `json:"board_id"`
	CodeHTML string `json:"code_html"`
	CodeCSS  string `json:"code_css"`
	Type     string `json:"type"`
}