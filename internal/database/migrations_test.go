package database

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMigrationTransaction(t *testing.T) {
	for _, failed := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS boards").WillReturnResult(sqlmock.NewResult(0, 0))
		if failed {
			mock.ExpectExec("ALTER TABLE boards").WillReturnError(errors.New("migration failed"))
			mock.ExpectRollback()
		} else {
			mock.ExpectExec("ALTER TABLE boards").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
		}
		err = (&DB{db}).MigrateContext(context.Background())
		if (err != nil) != failed {
			t.Fatalf("failed=%v err=%v", failed, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
