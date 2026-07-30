package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Casalis/LedgerFun/internal/api"
	"github.com/Casalis/LedgerFun/internal/store"
	"github.com/gofor-little/env"
)

func main() {
	if err := env.Load(".env2"); err != nil {
		panic(err)
	}

	conn, err := store.Connect()
	if err != nil {
		fmt.Printf("failed to connect to DB : %d", err)
		return
	}
	defer conn.Close()

	s := store.New(conn)
	h := api.NewHandler(s)

	addr := env.Get("LISTEN_ADDR", ":8080")
	server := &http.Server{
		Addr:    addr,
		Handler: h.Routes(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	stop()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
}
