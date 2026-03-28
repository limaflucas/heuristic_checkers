package main

import (
	"log"
	"os"

	"github.com/limaflucas/heuristic_checkers/internal/api"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
	"github.com/limaflucas/heuristic_checkers/internal/manager"
)

func main() {
	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}

	store := engine.NewGameStore()
	mgr := manager.NewManager(store)
	srv := api.NewServer(addr, store, mgr)

	log.Printf("Checkers API server starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
