package trade

import (
	"CarBN/common"
	"CarBN/feed"
	"CarBN/subscription"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Service struct {
	db                  *pgxpool.Pool
	feed                *feed.Service
	subscriptionService *subscription.SubscriptionService
}

func NewService(db *pgxpool.Pool, feedSvc *feed.Service, subscriptionSvc *subscription.SubscriptionService) *Service {
	return &Service{
		db:                  db,
		feed:                feedSvc,
		subscriptionService: subscriptionSvc,
	}
}

// CreateTrade creates a new trade request between users
func (s *Service) CreateTrade(ctx context.Context, userIDFrom, userIDTo int, userFromCarIDs, userToCarIDs []int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Starting trade creation: from user %d to user %d", userIDFrom, userIDTo)

	// Check subscription status for both users
	hasSubscriptionFrom, err := s.subscriptionService.HasActiveSubscription(ctx, userIDFrom)
	if err != nil {
		return fmt.Errorf("failed to check from user subscription: %w", err)
	}
	if !hasSubscriptionFrom {
		return fmt.Errorf("trading requires an active subscription")
	}

	hasSubscriptionTo, err := s.subscriptionService.HasActiveSubscription(ctx, userIDTo)
	if err != nil {
		return fmt.Errorf("failed to check to user subscription: %w", err)
	}
	if !hasSubscriptionTo {
		return fmt.Errorf("cannot trade with a user without an active subscription")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("Failed to begin transaction: %v", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if a pending trade already exists with the same parameters
	var existingTradeCount int
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM trades 
		WHERE status = 'pending' 
		AND user_id_from = $1 
		AND user_id_to = $2 
		AND user_from_user_car_ids = $3 
		AND user_to_user_car_ids = $4
	`, userIDFrom, userIDTo, userFromCarIDs, userToCarIDs).Scan(&existingTradeCount)
	if err != nil {
		logger.Printf("Failed to check for existing trades: %v", err)
		return fmt.Errorf("failed to check for existing trades: %w", err)
	}

	if existingTradeCount > 0 {
		logger.Printf("Pending trade already exists with the same parameters")
		return tx.Commit(ctx) // Return success as this is not an error condition
	}

	logger.Printf("Verifying car ownership for user %d", userIDFrom)
	if err := s.verifyCarOwnership(ctx, tx, userIDFrom, userFromCarIDs); err != nil {
		logger.Printf("Car ownership verification failed for user %d: %v", userIDFrom, err)
		return fmt.Errorf("failed to verify from user car ownership: %w", err)
	}

	logger.Printf("Verifying car ownership for user %d", userIDTo)
	if err := s.verifyCarOwnership(ctx, tx, userIDTo, userToCarIDs); err != nil {
		logger.Printf("Car ownership verification failed for user %d: %v", userIDTo, err)
		return fmt.Errorf("failed to verify to user car ownership: %w", err)
	}

	logger.Printf("Creating trade record in database")
	_, err = tx.Exec(ctx, `
		INSERT INTO trades (user_id_from, user_id_to, status, user_from_user_car_ids, user_to_user_car_ids)
		VALUES ($1, $2, 'pending', $3, $4)
	`, userIDFrom, userIDTo, userFromCarIDs, userToCarIDs)
	if err != nil {
		logger.Printf("Failed to insert trade record: %v", err)
		return fmt.Errorf("failed to create trade: %w", err)
	}

	logger.Printf("Trade creation successful, committing transaction")
	return tx.Commit(ctx)
}

// AcceptTrade processes a trade acceptance
func (s *Service) AcceptTrade(ctx context.Context, userID int, tradeID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Starting trade acceptance process for trade ID %d", tradeID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("Failed to begin transaction: %v", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get trade details
	var userIDFrom, userIDTo int
	var userFromCarIDs, userToCarIDs []int
	var status string

	logger.Printf("Fetching trade details for trade ID %d", tradeID)
	err = tx.QueryRow(ctx, `
		SELECT user_id_from, user_id_to, status, user_from_user_car_ids, user_to_user_car_ids
		FROM trades
		WHERE id = $1
	`, tradeID).Scan(&userIDFrom, &userIDTo, &status, &userFromCarIDs, &userToCarIDs)
	if err != nil {
		logger.Printf("Failed to fetch trade details: %v", err)
		return fmt.Errorf("failed to get trade details: %w", err)
	}

	if userID != userIDTo {
		logger.Printf("User %d is not the recipient of trade %d", userID, tradeID)
		return fmt.Errorf("user is not the recipient of trade")
	}

	logger.Printf("Verifying ownership of cars for user %d", userIDFrom)
	if err := s.verifyCarOwnership(ctx, tx, userIDFrom, userFromCarIDs); err != nil {
		logger.Printf("Car ownership verification failed for user %d: %v", userIDFrom, err)
		return fmt.Errorf("failed to verify from user car ownership: %w", err)
	}

	logger.Printf("Verifying ownership of cars for user %d", userIDTo)
	if err := s.verifyCarOwnership(ctx, tx, userIDTo, userToCarIDs); err != nil {
		logger.Printf("Car ownership verification failed for user %d: %v", userIDTo, err)
		return fmt.Errorf("failed to verify to user car ownership: %w", err)
	}

	if status != "pending" {
		logger.Printf("Invalid trade status: %s", status)
		return fmt.Errorf("trade is not pending")
	}

	logger.Printf("Updating car ownerships for trade ID %d", tradeID)
	if err := s.updateCarOwnerships(ctx, tx, userIDFrom, userIDTo, userFromCarIDs, userToCarIDs); err != nil {
		logger.Printf("Failed to update car ownerships: %v", err)
		return fmt.Errorf("failed to update car ownerships: %w", err)
	}

	logger.Printf("Updating trade status to accepted")
	if _, err := tx.Exec(ctx, `
		UPDATE trades
		SET status = 'accepted',
		traded_at = NOW()
		WHERE id = $1
	`, tradeID); err != nil {
		logger.Printf("Failed to update trade status: %v", err)
		return fmt.Errorf("failed to update trade status: %w", err)
	}

	// Auto-decline other pending trades involving these cars
	logger.Printf("Auto-declining other trades involving the traded cars")
	if err := s.declineTradesWithCars(ctx, tx, tradeID, append(userFromCarIDs, userToCarIDs...)); err != nil {
		logger.Printf("Failed to auto-decline related trades: %v", err)
		return fmt.Errorf("failed to auto-decline related trades: %w", err)
	}

	// Create feed entries for both users
	if err := s.feed.CreateFeed(ctx, userIDFrom, "trade_completed", tradeID, userIDTo); err != nil {
		logger.Printf("Failed to create feed entry for user %d: %v", userIDFrom, err)
		return fmt.Errorf("failed to create feed entry: %w", err)
	}

	logger.Printf("Trade acceptance successful, committing transaction")
	return tx.Commit(ctx)
}

// DeclineTrade marks a trade as declined
func (s *Service) DeclineTrade(ctx context.Context, tradeID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Declining trade ID %d", tradeID)

	result, err := s.db.Exec(ctx, `
		UPDATE trades
		SET status = 'declined'
		WHERE id = $1 AND status = 'pending'
	`, tradeID)
	if err != nil {
		logger.Printf("Failed to decline trade: %v", err)
		return fmt.Errorf("failed to decline trade: %w", err)
	}

	if result.RowsAffected() == 0 {
		logger.Printf("No pending trade found with ID %d", tradeID)
		return fmt.Errorf("no pending trade found with ID %d", tradeID)
	}

	logger.Printf("Trade %d declined successfully", tradeID)
	return nil
}

// Helper function to decline trades involving specific cars
func (s *Service) declineTradesWithCars(ctx context.Context, tx pgx.Tx, excludeTradeID int, carIDs []int) error {
	if len(carIDs) == 0 {
		return nil
	}

	_, err := tx.Exec(ctx, `
		UPDATE trades
		SET status = 'declined'
		WHERE id != $1
		AND status = 'pending'
		AND (
			EXISTS (
				SELECT 1
				FROM unnest(user_from_user_car_ids) car_id
				WHERE car_id = ANY($2)
			)
			OR
			EXISTS (
				SELECT 1
				FROM unnest(user_to_user_car_ids) car_id
				WHERE car_id = ANY($2)
			)
		)
	`, excludeTradeID, carIDs)

	if err != nil {
		return fmt.Errorf("failed to decline related trades: %w", err)
	}

	return nil
}

// Helper functions

func (s *Service) verifyCarOwnership(ctx context.Context, tx pgx.Tx, userID int, carIDs []int) error {
	for _, carID := range carIDs {
		var count int
		err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM user_cars
			WHERE id = $1 AND user_id = $2
		`, carID, userID).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to verify car ownership: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("user %d does not own car %d", userID, carID)
		}
	}
	return nil
}

func (s *Service) updateCarOwnerships(ctx context.Context, tx pgx.Tx, userIDFrom, userIDTo int, userFromCarIDs, userToCarIDs []int) error {
	// Update ownership of cars from userIDFrom to userIDTo
	if err := s.updateOwnership(ctx, tx, userFromCarIDs, userIDTo); err != nil {
		return fmt.Errorf("failed to update from user cars: %w", err)
	}

	// Update ownership of cars from userIDTo to userIDFrom
	if err := s.updateOwnership(ctx, tx, userToCarIDs, userIDFrom); err != nil {
		return fmt.Errorf("failed to update to user cars: %w", err)
	}

	return nil
}

func (s *Service) updateOwnership(ctx context.Context, tx pgx.Tx, carIDs []int, newUserID int) error {
	for _, carID := range carIDs {
		_, err := tx.Exec(ctx, `
			UPDATE user_cars
			SET user_id = $1
			WHERE id = $2
		`, newUserID, carID)
		if err != nil {
			return fmt.Errorf("failed to update car ownership: %w", err)
		}
	}
	return nil
}

type TradeInfo struct {
	ID             int     `json:"id"`
	UserIDFrom     int     `json:"user_id_from"`
	UserIDTo       int     `json:"user_id_to"`
	Status         string  `json:"status"`
	UserFromCarIDs []int   `json:"user_from_user_car_ids"`
	UserToCarIDs   []int   `json:"user_to_user_car_ids"`
	CreatedAt      string  `json:"created_at"`
	TradedAt       *string `json:"traded_at,omitempty"`
}

// GetUserTrades retrieves all trades a user is involved in, with pagination
func (s *Service) GetUserTrades(ctx context.Context, userID int, page, pageSize int) ([]TradeInfo, int, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching trades for user %d, page %d, page size %d", userID, page, pageSize)

	offset := (page - 1) * pageSize

	// Get total count first
	var totalCount int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trades
		WHERE user_id_from = $1 OR user_id_to = $1
	`, userID).Scan(&totalCount)
	if err != nil {
		logger.Printf("Failed to get total trade count: %v", err)
		return nil, 0, fmt.Errorf("failed to get total trade count: %w", err)
	}

	// Get paginated trades
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id_from, user_id_to, status, user_from_user_car_ids, user_to_user_car_ids, created_at, traded_at
		FROM trades
		WHERE user_id_from = $1 OR user_id_to = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, offset)
	if err != nil {
		logger.Printf("Failed to fetch trades: %v", err)
		return nil, 0, fmt.Errorf("failed to fetch trades: %w", err)
	}
	defer rows.Close()

	var trades []TradeInfo
	for rows.Next() {
		var trade TradeInfo
		var createdAt time.Time
		var tradedAt *time.Time
		err := rows.Scan(
			&trade.ID,
			&trade.UserIDFrom,
			&trade.UserIDTo,
			&trade.Status,
			&trade.UserFromCarIDs,
			&trade.UserToCarIDs,
			&createdAt,
			&tradedAt,
		)
		if err != nil {
			logger.Printf("Failed to scan trade row: %v", err)
			return nil, 0, fmt.Errorf("failed to scan trade row: %w", err)
		}
		trade.CreatedAt = common.FormatTimestamp(createdAt)
		if tradedAt != nil {
			tradedAtStr := common.FormatTimestamp(*tradedAt)
			trade.TradedAt = &tradedAtStr
		}
		trades = append(trades, trade)
	}

	if err = rows.Err(); err != nil {
		logger.Printf("Error iterating trade rows: %v", err)
		return nil, 0, fmt.Errorf("error iterating trade rows: %w", err)
	}

	logger.Printf("Successfully fetched %d trades for user %d", len(trades), userID)
	return trades, totalCount, nil
}

// GetTradeByID retrieves a specific trade by its ID
func (s *Service) GetTradeByID(ctx context.Context, tradeID int) (*TradeInfo, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching trade with ID %d", tradeID)

	var trade TradeInfo
	var createdAt time.Time
	var tradedAt *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id_from, user_id_to, status, user_from_user_car_ids, user_to_user_car_ids, created_at, traded_at
		FROM trades
		WHERE id = $1
	`, tradeID).Scan(
		&trade.ID,
		&trade.UserIDFrom,
		&trade.UserIDTo,
		&trade.Status,
		&trade.UserFromCarIDs,
		&trade.UserToCarIDs,
		&createdAt,
		&tradedAt,
	)

	if err == pgx.ErrNoRows {
		logger.Printf("No trade found with ID %d", tradeID)
		return nil, nil
	}
	if err != nil {
		logger.Printf("Failed to fetch trade: %v", err)
		return nil, fmt.Errorf("failed to fetch trade: %w", err)
	}

	trade.CreatedAt = common.FormatTimestamp(createdAt)
	if tradedAt != nil {
		tradedAtStr := common.FormatTimestamp(*tradedAt)
		trade.TradedAt = &tradedAtStr
	}

	return &trade, nil
}
