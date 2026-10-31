package database

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func boardRows(revision int64, html, css string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "revision", "code_html", "code_css", "created_at", "updated_at"}).
		AddRow("board-id", revision, html, css, time.Unix(0, 0), time.Unix(1, 0))
}

func TestSaveBoardConditionalWrite(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db := &DB{sqlDB}
	query := "UPDATE boards SET code_html = $1, code_css = $2, revision = revision + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $3 AND revision = $4 RETURNING " + boardColumns
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("", "new CSS", "board-id", int64(7)).WillReturnRows(boardRows(8, "", "new CSS"))
	board, err := db.SaveBoard(context.Background(), "board-id", 7, "", "new CSS")
	if err != nil || board.Revision != 8 || board.CodeHTML != "" {
		t.Fatalf("board=%+v err=%v", board, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveBoardConflictReturnsCurrentDocumentWithoutAnotherWrite(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db := &DB{sqlDB}
	mock.ExpectQuery("UPDATE boards").WithArgs("stale", "", "board-id", int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM boards WHERE id").WithArgs("board-id").WillReturnRows(boardRows(2, "other writer", "CSS"))
	board, err := db.SaveBoard(context.Background(), "board-id", 1, "stale", "")
	if !errors.Is(err, ErrConflict) || board.Revision != 2 || board.CodeHTML != "other writer" {
		t.Fatalf("board=%+v err=%v", board, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveMissingBoardDoesNotInventAConflict(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db := &DB{sqlDB}
	mock.ExpectQuery("UPDATE boards").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)
	_, err = db.SaveBoard(context.Background(), "missing", 0, "", "")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageFailureDoesNotReturnAFalseConflict(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db := &DB{sqlDB}
	failure := errors.New("database unavailable")
	mock.ExpectQuery("UPDATE boards").WillReturnError(failure)
	_, err = db.SaveBoard(context.Background(), "id", 0, "", "")
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
