package http

import (
	"fmt"
	"log"
	stdhttp "net/http"

	"iamstagram_22520060/internal/config"
	"iamstagram_22520060/internal/database"
)

func Run() error {
	// 1. Load Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Connect to Database
	db, err := database.Connect(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// 3. Setup Server
	log.Println("Starting server on :8080")
	// TODO: Add handlers here

	return stdhttp.ListenAndServe(":8080", nil)
}
