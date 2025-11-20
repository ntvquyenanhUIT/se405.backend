package main

import (
	"log"

	"iamstagram_22520060/internal/transport/http"
)

func main() {
	if err := http.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
