package database

import (
	"fmt"
	"log"

	"iamstagram_22520060/internal/config"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
)

func Connect(cfg *config.Config) (*sqlx.DB, error) {

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=require",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Connected to database successfully")
	log.Println("Hello world")
	return db, nil
}
