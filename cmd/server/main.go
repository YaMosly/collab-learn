package main

import (
	"collab-learn/internal/access"
	"collab-learn/internal/config"
	"collab-learn/internal/database"
	"collab-learn/internal/handlers"
	"collab-learn/internal/health"
	"collab-learn/internal/redis"
	"collab-learn/internal/websocket"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "healthcheck" {
			return fmt.Errorf("usage: collablearn [healthcheck]")
		}
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(cfg.ProbeURL)
		if err != nil {
			return fmt.Errorf("readiness probe failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server is not ready")
		}
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()

	log.Println("Starting CollabLearn server...")

	var db *database.DB
	if err := retry(startup, "PostgreSQL", func(ctx context.Context) error {
		var err error
		db, err = database.Open(ctx, cfg.DatabaseURL)
		return err
	}); err != nil {
		return err
	}
	defer db.Close()

	if err := db.MigrateContext(startup); err != nil {
		return fmt.Errorf("required database migration failed: %w", err)
	}

	var redisClient *redis.Client
	if err := retry(startup, "Redis", func(ctx context.Context) error {
		var err error
		redisClient, err = redis.Connect(ctx, cfg.RedisAddress)
		return err
	}); err != nil {
		return err
	}
	defer redisClient.Close()

	hub := websocket.NewHub(redisClient)
	defer hub.Close()
	if err := retry(startup, "Redis subscription", hub.StartContext); err != nil {
		return err
	}
	cancel()

	r := chi.NewRouter()

	r.Use(access.Headers)
	r.Use(access.Logger)
	r.Use(middleware.Recoverer)

	boardHandler := handlers.NewBoardHandler(db, redisClient, hub.BroadcastBoard)
	wsHandler := websocket.NewWSHandler(hub, db, cfg.PublicOrigin)
	status := &health.Handler{Database: db.PingContext, Redis: func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }, Hub: hub.Active}
	r.Get("/health", status.Live)
	r.Get("/livez", status.Live)
	r.Get("/readyz", status.Ready)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, "web/static/index.html")
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	r.Route("/api", func(r chi.Router) {
		r.Use(access.SameOrigin(cfg.PublicOrigin))
		r.Use(access.Limit(redisClient, access.API))
		r.With(access.Limit(redisClient, access.Create)).Post("/boards", boardHandler.CreateBoard)
		r.Get("/boards/{id}", boardHandler.GetBoard)
		r.Put("/boards/{id}", boardHandler.UpdateBoard)
		r.With(access.Limit(redisClient, access.Socket)).Get("/boards/{id}/ws", wsHandler.HandleWebSocket)
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddress,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverError := make(chan error, 1)
	go func() { serverError <- srv.ListenAndServe() }()
	log.Printf("Server listening on %s", cfg.HTTPAddress)
	select {
	case err := <-serverError:
		if err != http.ErrServerClosed {
			return fmt.Errorf("HTTP server failed: %w", err)
		}
	case <-ctx.Done():
	}
	log.Println("Shutting down server...")
	status.Stopping.Store(true)
	hub.Close()

	shutdown, finish := context.WithTimeout(context.Background(), 30*time.Second)
	defer finish()

	if err := srv.Shutdown(shutdown); err != nil {
		srv.Close()
		return fmt.Errorf("HTTP shutdown: %w", err)
	}

	log.Println("Server exited")
	return nil
}

// Dependency errors can include credentials. Report the dependency and deadline,
// but never log the connection string or the raw authentication error.
func retry(ctx context.Context, dependency string, connect func(context.Context) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s startup: %w", dependency, err)
		}
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := connect(attempt)
		cancel()
		if err == nil {
			return nil
		}
		log.Printf("Waiting for %s...", dependency)
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%s startup: %w", dependency, ctx.Err())
		case <-timer.C:
		}
	}
}
