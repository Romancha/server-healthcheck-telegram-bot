package healthcheck

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type response struct {
	Status   string `json:"status"`
	Telegram string `json:"telegram,omitempty"`
}

// Start starts the health check HTTP server on the given address.
// It blocks until the context is cancelled, then gracefully shuts down.
func Start(ctx context.Context, addr string, bot *tgbotapi.BotAPI) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check Telegram API connectivity
		_, err := bot.GetMe()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response{
				Status:   "error",
				Telegram: err.Error(),
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{Status: "ok"})
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Printf("[INFO] Shutting down health check server")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("[ERROR] Health check server shutdown error: %v", err)
		}
	}()

	log.Printf("[INFO] Health check server starting on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
