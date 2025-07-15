package postgres

import (
	"CarBN/common"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var DB *pgxpool.Pool

func InitDB(ctx context.Context) error {
	// Create context with timeout for initialization
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	DB_USER := os.Getenv("DB_USER")
	DB_PASSWORD := os.Getenv("DB_PASSWORD")
	DB_NAME := os.Getenv("DB_NAME")
	POSTGRES_SSL_MODE := os.Getenv("POSTGRES_SSL_MODE")

	psqlInfo := fmt.Sprintf("postgresql://%s:%s@localhost:5432/%s?sslmode=%s",
		DB_USER, DB_PASSWORD, DB_NAME, POSTGRES_SSL_MODE)

	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Attempting to connect to database with connection string: %s", psqlInfo)

	config, err := pgxpool.ParseConfig(psqlInfo)
	if err != nil {
		return fmt.Errorf("unable to parse connection string: %w", err)
	}

	// Configure connection pool
	config.MaxConns = 50
	config.MinConns = 10
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute * 30
	config.HealthCheckPeriod = time.Minute
	config.ConnConfig.ConnectTimeout = 10 * time.Second

	DB, err = pgxpool.ConnectConfig(initCtx, config)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}

	// Verify connection with separate context
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := DB.Ping(pingCtx); err != nil {
		DB.Close()
		return fmt.Errorf("unable to ping database: %w", err)
	}

	logger.Printf("Successfully connected to database with pool of %d-%d connections", config.MinConns, config.MaxConns)
	return nil
}

// CloseDB gracefully closes the database connection pool
func CloseDB(ctx context.Context) {
	if DB != nil {
		// Create context for graceful shutdown
		closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Wait for queries to finish
		DB.Close()

		select {
		case <-closeCtx.Done():
			logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
			logger.Printf("Warning: Database shutdown timed out after 10 seconds")
		default:
			logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
			logger.Printf("Database connection pool closed successfully")
		}
	}
}

// WithTransaction executes a function within a transaction
func WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback if panic occurs
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("error: %v, rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
