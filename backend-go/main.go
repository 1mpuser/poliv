package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"poliv/internal/api"
	"poliv/internal/config"
	"poliv/internal/db"
	"poliv/internal/light"
	"poliv/internal/migrate"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			runServer()
			return
		case "healthcheck":
			os.Exit(runHealthcheck())
			return
		case "set-owner", "create-user", "reset-password", "make-admin":
			os.Exit(runCLI(os.Args[1:]))
			return
		}
	}
	runServer()
}

func runHealthcheck() int {
	cfg, err := config.Load()
	if err != nil {
		return 1
	}
	dbConn, err := db.New(context.Background(), cfg.DatabaseURL, cfg.Zone)
	if err != nil {
		return 1
	}
	defer dbConn.Close()
	return 0
}

func runServer() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbConn, err := db.New(ctx, cfg.DatabaseURL, cfg.Zone)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	if err := migrate.New(dbConn.Pool).Up(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}

	srv := api.New(dbConn.Pool, cfg.Zone, cfg.JWTSecret, cfg.JWTExpireDays)
	httpServer := &http.Server{
		Addr:    ":8000",
		Handler: srv.Handler(),
	}

	// фоновые задачи: шаг ламп раз в минуту, свет по городу раз в 30 минут
	go background(ctx, dbConn, srv)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "http:", err)
			stop()
		}
	}()
	fmt.Println("Поливалка API запущена на :8000")

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func background(ctx context.Context, dbConn *db.DB, srv *api.Server) {
	tickTicker := time.NewTicker(time.Minute)
	syncTicker := time.NewTicker(30 * time.Minute)
	defer tickTicker.Stop()
	defer syncTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tickTicker.C:
			_ = srv.Lamps.Tick(ctx, now.UTC())
		case <-syncTicker.C:
			_ = light.SyncAll(ctx, dbConn.Pool, dbConn.Zone)
		}
	}
}
