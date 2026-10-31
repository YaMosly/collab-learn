package handlers

import (
	"collab-learn/internal/database"
	"collab-learn/internal/models"
	"collab-learn/internal/redis"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type BoardRepository interface {
	CreateBoard(context.Context, string) (*models.Board, error)
	GetBoard(context.Context, string) (*models.Board, error)
	SaveBoard(context.Context, string, int64, string, string) (*models.Board, error)
}

type BoardHandler struct {
	db        BoardRepository
	redis     *redis.Client
	broadcast func(*models.Board)
}

func NewBoardHandler(db BoardRepository, redis *redis.Client, broadcast func(*models.Board)) *BoardHandler {
	return &BoardHandler{db: db, redis: redis, broadcast: broadcast}
}

func boardResponse(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func boardError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Board not found", http.StatusNotFound)
		return
	}
	log.Printf("Board storage error: %v", err)
	http.Error(w, "Board storage unavailable", http.StatusInternalServerError)
}

func (h *BoardHandler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	board, err := h.db.CreateBoard(r.Context(), uuid.NewString())
	if err != nil {
		boardError(w, err)
		return
	}
	boardResponse(w, http.StatusCreated, board)
}

func (h *BoardHandler) GetBoard(w http.ResponseWriter, r *http.Request) {
	// Reconnect must read the authoritative revision, never a stale cache entry.
	board, err := h.db.GetBoard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		boardError(w, err)
		return
	}
	boardResponse(w, http.StatusOK, board)
}

func (h *BoardHandler) UpdateBoard(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var update models.BoardWrite
	if err := decoder.Decode(&update); err != nil || !update.Valid() {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "Document exceeds 512 KiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Expected base_revision, code_html and code_css", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "Document exceeds 512 KiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Expected one JSON document", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	board, err := h.db.SaveBoard(r.Context(), id, *update.BaseRevision, *update.CodeHTML, *update.CodeCSS)
	if errors.Is(err, database.ErrConflict) {
		boardResponse(w, http.StatusConflict, board)
		return
	}
	if err != nil {
		boardError(w, err)
		return
	}
	if h.broadcast != nil {
		h.broadcast(board)
	}
	if h.redis != nil {
		if err := h.redis.PublishBoardUpdate(id, board); err != nil {
			log.Printf("Board publish failed: %v", err)
		}
	}
	boardResponse(w, http.StatusOK, board)
}
