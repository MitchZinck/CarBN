package friends

import (
	"CarBN/common"
	"CarBN/feed"
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v4/pgxpool"
)

type Service struct {
	db   *pgxpool.Pool
	feed *feed.Service
}

func NewService(db *pgxpool.Pool, feedSvc *feed.Service) *Service {
	return &Service{
		db:   db,
		feed: feedSvc,
	}
}

func (s *Service) SendFriendRequest(ctx context.Context, userID, friendID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	_, err := s.db.Exec(ctx, `
		INSERT INTO friends (user_id, friend_id, status)
		VALUES ($1, $2, 'pending')
		ON CONFLICT (user_id, friend_id) DO NOTHING
	`, userID, friendID)

	if err != nil {
		logger.Printf("Failed to send friend request from user %d to user %d: %v", userID, friendID, err)
		return fmt.Errorf("failed to send friend request: %w", err)
	}

	logger.Printf("Friend request sent successfully from user %d to user %d", userID, friendID)
	return nil
}

func (s *Service) AcceptFriendRequest(ctx context.Context, requestID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("Failed to begin transaction for accepting friend request %d: %v", requestID, err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID, friendID int
	err = tx.QueryRow(ctx, `
		SELECT user_id, friend_id
		FROM friends
		WHERE id = $1
	`, requestID).Scan(&userID, &friendID)
	if err != nil {
		logger.Printf("Failed to fetch friend request details for ID %d: %v", requestID, err)
		return fmt.Errorf("failed to fetch friend request: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE friends
		SET status = 'accepted'
		WHERE id = $1
	`, requestID)
	if err != nil {
		logger.Printf("Failed to update friend request status for ID %d: %v", requestID, err)
		return fmt.Errorf("failed to update friend request: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Printf("Failed to commit transaction for friend request %d: %v", requestID, err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Create feed entries for both users after transaction is committed
	if err := s.feed.CreateFeed(ctx, userID, "friend_accepted", requestID, friendID); err != nil {
		logger.Printf("Failed to create feed entry for user %d: %v", userID, err)
		return fmt.Errorf("failed to create feed entry: %w", err)
	}

	logger.Printf("Friend request %d accepted successfully between users %d and %d", requestID, userID, friendID)
	return nil
}

func (s *Service) RejectFriendRequest(ctx context.Context, requestID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	_, err := s.db.Exec(ctx, `
		UPDATE friends
		SET status = 'rejected'
		WHERE id = $1
	`, requestID)

	if err != nil {
		logger.Printf("Failed to reject friend request %d: %v", requestID, err)
		return fmt.Errorf("failed to reject friend request: %w", err)
	}

	logger.Printf("Friend request %d rejected successfully", requestID)
	return nil
}

func (s *Service) CheckFriendship(ctx context.Context, userID, friendID int) (bool, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM friends 
			WHERE ((user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1))
			AND status = 'accepted'
		)
	`, userID, friendID).Scan(&exists)

	if err != nil {
		logger.Printf("Failed to check friendship between users %d and %d: %v", userID, friendID, err)
		return false, fmt.Errorf("failed to check friendship: %w", err)
	}

	return exists, nil
}

type Friend struct {
	ID             int     `json:"id"`
	DisplayName    *string `json:"display_name"`
	ProfilePicture *string `json:"profile_picture"`
}

type PaginatedFriends struct {
	Friends []Friend `json:"friends"`
	Total   int      `json:"total"`
	HasMore bool     `json:"has_more"`
}

func (s *Service) GetFriends(ctx context.Context, userID int, limit int, offset int) (PaginatedFriends, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching friends for user %d with limit %d and offset %d", userID, limit, offset)

	// Set default limit if not provided or invalid
	if limit <= 0 {
		limit = 10
	}

	// Ensure offset is not negative
	if offset < 0 {
		offset = 0
	}

	// Get total count first
	var total int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM friends f
		WHERE (f.user_id = $1 OR f.friend_id = $1)
		AND f.status = 'accepted'
	`, userID).Scan(&total)
	if err != nil {
		logger.Printf("Failed to get total friends count for user %d: %v", userID, err)
		return PaginatedFriends{}, fmt.Errorf("failed to get total count: %w", err)
	}

	query := `
		SELECT u.id, u.display_name, u.profile_picture
		FROM friends f
		JOIN users u ON (f.friend_id = u.id AND f.user_id = $1) 
			OR (f.user_id = u.id AND f.friend_id = $1)
		WHERE f.status = 'accepted'
		ORDER BY u.display_name
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		logger.Printf("Failed to fetch friends for user %d: %v", userID, err)
		return PaginatedFriends{}, fmt.Errorf("failed to fetch friends: %w", err)
	}
	defer rows.Close()

	var friends []Friend
	for rows.Next() {
		var friend Friend
		if err := rows.Scan(&friend.ID, &friend.DisplayName, &friend.ProfilePicture); err != nil {
			logger.Printf("Error scanning friend row: %v", err)
			return PaginatedFriends{}, fmt.Errorf("error scanning friend data: %w", err)
		}
		friends = append(friends, friend)
	}

	if err := rows.Err(); err != nil {
		logger.Printf("Error iterating friend rows: %v", err)
		return PaginatedFriends{}, fmt.Errorf("error iterating friends: %w", err)
	}

	hasMore := (offset + len(friends)) < total

	logger.Printf("Successfully retrieved %d friends for user %d (total: %d)", len(friends), userID, total)
	return PaginatedFriends{
		Friends: friends,
		Total:   total,
		HasMore: hasMore,
	}, nil
}
