package handlers

import (
	"collab-learn/internal/database"
	"collab-learn/internal/models"
	"collab-learn/internal/redis"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type BoardHandler struct {
	db    *database.DB
	redis *redis.Client
}

func NewBoardHandler(db *database.DB, redis *redis.Client) *BoardHandler {
	return &BoardHandler{
		db:    db,
		redis: redis,
	}
}

func (h *BoardHandler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	board := &models.Board{
		ID:        uuid.New().String(),
		CodeHTML:  "",
		CodeCSS:   "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	query := `INSERT INTO boards (id, code_html, code_css, created_at, updated_at) 
			  VALUES ($1, $2, $3, $4, $5)`
	
	_, err := h.db.Exec(query, board.ID, board.CodeHTML, board.CodeCSS, board.CreatedAt, board.UpdatedAt)
	if err != nil {
		http.Error(w, "Failed to create board", http.StatusInternalServerError)
		log.Printf("Error creating board: %v", err)
		return
	}

	h.redis.CacheBoard(board.ID, board, 15*time.Minute)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(board)
}

func (h *BoardHandler) GetBoard(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")

	var board models.Board
	if err := h.redis.GetCachedBoard(boardID, &board); err == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(board)
		return
	}

	query := `SELECT id, code_html, code_css, created_at, updated_at 
			  FROM boards WHERE id = $1`
	
	row := h.db.QueryRow(query, boardID)
	err := row.Scan(&board.ID, &board.CodeHTML, &board.CodeCSS, &board.CreatedAt, &board.UpdatedAt)
	if err != nil {
		http.Error(w, "Board not found", http.StatusNotFound)
		return
	}

	h.redis.CacheBoard(board.ID, board, 15*time.Minute)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(board)
}

func (h *BoardHandler) UpdateBoard(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")

	var update struct {
		CodeHTML string `json:"code_html"`
		CodeCSS  string `json:"code_css"`
	}

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := `UPDATE boards SET code_html = $1, code_css = $2, updated_at = $3 
			  WHERE id = $4`
	
	_, err := h.db.Exec(query, update.CodeHTML, update.CodeCSS, time.Now(), boardID)
	if err != nil {
		http.Error(w, "Failed to update board", http.StatusInternalServerError)
		log.Printf("Error updating board: %v", err)
		return
	}

	boardUpdate := models.BoardUpdate{
		BoardID:  boardID,
		CodeHTML: update.CodeHTML,
		CodeCSS:  update.CodeCSS,
		Type:     "update",
	}

	h.redis.PublishBoardUpdate(boardID, boardUpdate)

	query = `SELECT id, code_html, code_css, created_at, updated_at 
			 FROM boards WHERE id = $1`
	
	var board models.Board
	row := h.db.QueryRow(query, boardID)
	err = row.Scan(&board.ID, &board.CodeHTML, &board.CodeCSS, &board.CreatedAt, &board.UpdatedAt)
	if err != nil {
		http.Error(w, "Failed to fetch updated board", http.StatusInternalServerError)
		return
	}

	h.redis.CacheBoard(board.ID, board, 15*time.Minute)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(board)
}

func (h *BoardHandler) ListBoards(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, code_html, code_css, created_at, updated_at 
			  FROM boards ORDER BY updated_at DESC LIMIT 50`
	
	rows, err := h.db.Query(query)
	if err != nil {
		http.Error(w, "Failed to fetch boards", http.StatusInternalServerError)
		log.Printf("Error fetching boards: %v", err)
		return
	}
	defer rows.Close()

	var boards []models.Board
	for rows.Next() {
		var board models.Board
		err := rows.Scan(&board.ID, &board.CodeHTML, &board.CodeCSS, &board.CreatedAt, &board.UpdatedAt)
		if err != nil {
			continue
		}
		boards = append(boards, board)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(boards)
}