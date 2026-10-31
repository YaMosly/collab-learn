package database

import (
	"collab-learn/internal/models"
	"context"
	"database/sql"
	"errors"
)

var ErrConflict = errors.New("board revision changed")

const boardColumns = "id, revision, code_html, code_css, created_at, updated_at"

func scanBoard(row interface{ Scan(...interface{}) error }) (*models.Board, error) {
	var board models.Board
	err := row.Scan(&board.ID, &board.Revision, &board.CodeHTML, &board.CodeCSS, &board.CreatedAt, &board.UpdatedAt)
	return &board, err
}

func (db *DB) GetBoard(ctx context.Context, id string) (*models.Board, error) {
	return scanBoard(db.QueryRowContext(ctx, "SELECT "+boardColumns+" FROM boards WHERE id = $1", id))
}

func (db *DB) CreateBoard(ctx context.Context, id string) (*models.Board, error) {
	return scanBoard(db.QueryRowContext(ctx, "INSERT INTO boards (id) VALUES ($1) RETURNING "+boardColumns, id))
}

// PostgreSQL evaluates the revision predicate and increments it in the same
// UPDATE, so two writers cannot both overwrite the same base revision.
func (db *DB) SaveBoard(ctx context.Context, id string, revision int64, html, css string) (*models.Board, error) {
	query := "UPDATE boards SET code_html = $1, code_css = $2, revision = revision + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $3 AND revision = $4 RETURNING " + boardColumns
	board, err := scanBoard(db.QueryRowContext(ctx, query, html, css, id, revision))
	if !errors.Is(err, sql.ErrNoRows) {
		return board, err
	}
	current, err := db.GetBoard(ctx, id)
	if err != nil {
		return nil, err
	}
	return current, ErrConflict
}
