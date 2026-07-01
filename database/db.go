package database

import (
	"context"
	"database/sql"
	"fmt"
)

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("could not close failing connection to database: %w", err)
		}
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	return db, nil
}

func Close(db *sql.DB) error {
	if err := db.Close(); err != nil {
		return fmt.Errorf("error closing database: %w", err)
	}
	return nil
}
