package api

import (
	"log"
	"net/http"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
	"github.com/limaflucas/heuristic_checkers/internal/manager"
)

// NewServer builds and returns a configured *http.Server.
func NewServer(addr string, store *engine.GameStore, mgr *manager.Manager) *http.Server {
	h := NewHandlers(store, mgr)
	mux := http.NewServeMux()

	// Exact match: list + create games.
	mux.HandleFunc("/api/v1/games", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListGames(w, r)
		case http.MethodPost:
			h.CreateGame(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Wildcard: game-specific endpoints.
	mux.HandleFunc("/api/v1/games/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path != "/api/v1/games/":
			h.DeleteGame(w, r)
		case r.Method == http.MethodPost && hasSuffix(r.URL.Path, "/moves"):
			h.MakeMove(w, r)
		case r.Method == http.MethodGet && hasSuffix(r.URL.Path, "/board"):
			h.GetBoard(w, r)
		case r.Method == http.MethodGet && hasSuffix(r.URL.Path, "/legal-moves"):
			h.GetLegalMoves(w, r)
		case r.Method == http.MethodGet && hasSuffix(r.URL.Path, "/events"):
			h.GetEvents(w, r)
		case r.Method == http.MethodGet && hasSuffix(r.URL.Path, "/moves"):
			h.GetMoves(w, r)
		case r.Method == http.MethodGet && hasSuffix(r.URL.Path, "/watch"):
			h.WatchGame(w, r)
		case r.Method == http.MethodGet && hasSuffix(r.URL.Path, "/stats"):
			h.GetGameStats(w, r)
		default:
			writeError(w, http.StatusNotFound, "route not found")
		}
	})

	// Manager endpoints.
	mux.HandleFunc("/api/v1/manager", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListManagerSessions(w, r)
		case http.MethodPost:
			h.StartManagerSession(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	
	mux.HandleFunc("/api/v1/manager/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.GetManagerSession(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	return &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(logMiddleware(mux)),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // no write timeout — SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
	}
}

func hasSuffix(path, suffix string) bool {
	n := len(path)
	s := len(suffix)
	return n >= s && path[n-s:] == suffix
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s — %v", r.Method, r.URL.Path, time.Since(start))
	})
}
