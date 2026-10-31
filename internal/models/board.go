package models

import (
	"time"
)

type Board struct {
	ID        string    `json:"id"`
	Revision  int64     `json:"revision"`
	CodeHTML  string    `json:"code_html"`
	CodeCSS   string    `json:"code_css"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// A write is conditional on the revision read by this editor. Pointers distinguish
// an intentional empty document from a missing or null field.
type BoardWrite struct {
	BaseRevision *int64  `json:"base_revision"`
	CodeHTML     *string `json:"code_html"`
	CodeCSS      *string `json:"code_css"`
}

func (w BoardWrite) Valid() bool {
	return w.BaseRevision != nil && *w.BaseRevision >= 0 && w.CodeHTML != nil && w.CodeCSS != nil
}

type Session struct {
	ID          string    `json:"id"`
	BoardID     string    `json:"board_id"`
	UserToken   string    `json:"user_token"`
	ConnectedAt time.Time `json:"connected_at"`
}
