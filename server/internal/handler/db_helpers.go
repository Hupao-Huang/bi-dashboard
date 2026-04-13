package handler

import (
	"database/sql"
	"log"
	"net/http"
)

func writeDatabaseError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	log.Printf("database query failed: %v", err)
	writeError(w, http.StatusInternalServerError, "database query failed")
	return true
}

func queryRowsOrWriteError(w http.ResponseWriter, db *sql.DB, query string, args ...interface{}) (*sql.Rows, bool) {
	rows, err := db.Query(query, args...)
	if writeDatabaseError(w, err) {
		return nil, false
	}
	return rows, true
}
