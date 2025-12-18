package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := &http.Server{
		Addr:    ":4000",
		Handler: Routes(logger),
	}

	logger.Info("starting server", "addr", ":4000")
	err := server.ListenAndServe()
	if err != nil {
		logger.Error(err.Error())
		panic(err)
	}
}

func Routes(logger *slog.Logger) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{"message": "Ok"}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("failed to encode a JSON response", slog.String("error", err.Error()))
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	})

	return r
}
