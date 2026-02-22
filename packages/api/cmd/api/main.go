package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/agentqa/agentqa/packages/api/internal/auth"
	"github.com/agentqa/agentqa/packages/api/internal/config"
	"github.com/agentqa/agentqa/packages/api/internal/httpapi"
	"github.com/agentqa/agentqa/packages/api/internal/push"
	"github.com/agentqa/agentqa/packages/api/internal/store/postgres"
	"github.com/agentqa/agentqa/packages/api/internal/waiter"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "agentqa-api ", log.LstdFlags)

	if cfg.DatabaseURL == "" {
		logger.Fatal("DATABASE_URL is required")
	}

	apiKeyEncryptionKey, err := auth.ParseAPIKeyEncryptionKey(cfg.APIKeyEncryptionKey)
	if err != nil {
		logger.Fatal(err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		logger.Fatalf("db open: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := waitForDB(db, logger, cfg.DBConnectRetries, cfg.DBConnectDelay); err != nil {
		logger.Fatalf("db ping: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		logger.Fatalf("gorm open: %v", err)
	}

	if err := postgres.AutoMigrate(gormDB); err != nil {
		logger.Fatalf("db migrate: %v", err)
	}

	store := postgres.NewStore(gormDB)
	waitHub := waiter.NewHub()
	pushSender := push.New(cfg, logger)
	tokenService := auth.NewTokenService(cfg.JWTSecret)

	server := httpapi.NewServer(cfg, store, waitHub, pushSender, tokenService, logger, apiKeyEncryptionKey)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Printf("listening on %s", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}

func waitForDB(db *sql.DB, logger *log.Logger, retries int, delay time.Duration) error {
	attempts := retries
	if attempts <= 0 {
		attempts = 1
	}
	if delay <= 0 {
		delay = 2 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := db.Ping(); err != nil {
			lastErr = err
			if attempt < attempts {
				logger.Printf("db ping failed (attempt %d/%d): %v", attempt, attempts, err)
				time.Sleep(delay)
				continue
			}
			break
		}
		return nil
	}

	return lastErr
}
