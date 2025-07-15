package likes

import (
	"CarBN/common"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

type Like struct {
	ID         int    `json:"id"`
	UserID     int    `json:"user_id"`
	TargetID   int    `json:"target_id"`
	TargetType string `json:"target_type"`
	CreatedAt  string `json:"created_at"`
}

type PaginatedLikes struct {
	Items      []Like `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

const (
	// Target types for likes
	TargetTypeFeedItem = "feed_item"
	TargetTypeUserCar  = "user_car"
)

// SQL queries
const (
	createLikeSQL = `
		INSERT INTO likes (user_id, target_id, target_type)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`

	deleteLikeSQL = `
		DELETE FROM likes
		WHERE user_id = $1 AND target_id = $2 AND target_type = $3`

	getFeedItemLikesSQL = `
		SELECT l.id, l.user_id, l.target_id, l.target_type, l.created_at
		FROM likes l
		WHERE l.target_id = $1 AND l.target_type = $2
		AND ($3::TIMESTAMPTZ IS NULL OR (l.created_at, l.id) < ($3::TIMESTAMPTZ, $4::INT))
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $5`

	getUserReceivedLikesSQL = `
        WITH feed_likes AS (
            SELECT l.id, l.user_id, l.target_id, l.target_type, l.created_at
            FROM likes l
            JOIN feed f ON l.target_id = f.id AND l.target_type = 'feed_item'
            WHERE f.user_id = $1 OR f.related_user_id = $1
        ),
        car_likes AS (
            SELECT l.id, l.user_id, l.target_id, l.target_type, l.created_at
            FROM likes l
            JOIN user_cars uc ON l.target_id = uc.id AND l.target_type = 'user_car'
            WHERE uc.user_id = $1
        ),
        combined_likes AS (
            SELECT * FROM feed_likes
            UNION ALL
            SELECT * FROM car_likes
        )
        SELECT id, user_id, target_id, target_type, created_at
        FROM combined_likes
        WHERE ($2::TIMESTAMPTZ IS NULL OR (created_at, id) < ($2::TIMESTAMPTZ, $3::INT))
        ORDER BY created_at DESC, id DESC
        LIMIT $4`

	getUserCarLikesSQL = `
		SELECT l.id, l.user_id, l.target_id, l.target_type, l.created_at
		FROM likes l
		WHERE l.target_id = $1 AND l.target_type = $2
		AND ($3::TIMESTAMPTZ IS NULL OR (l.created_at, l.id) < ($3::TIMESTAMPTZ, $4::INT))
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $5`

	checkUserLikedSQL = `
		SELECT EXISTS (
			SELECT 1 FROM likes 
			WHERE user_id = $1 AND target_id = $2 AND target_type = $3
		)`

	getLikesCountSQL = `
		SELECT COUNT(*) FROM likes 
		WHERE target_id = $1 AND target_type = $2`
)

func (s *Service) CreateLike(ctx context.Context, userID int, targetID int, targetType string) (*Like, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return nil, fmt.Errorf("logger not found in context")
	}

	var like Like
	var createdAt time.Time

	err := s.db.QueryRow(ctx, createLikeSQL, userID, targetID, targetType).Scan(&like.ID, &createdAt)
	if err != nil {
		logger.Printf("error creating like: %v", err)
		return nil, fmt.Errorf("creating like: %w", err)
	}

	like.UserID = userID
	like.TargetID = targetID
	like.TargetType = targetType
	like.CreatedAt = common.FormatTimestamp(createdAt)

	return &like, nil
}

func (s *Service) DeleteLike(ctx context.Context, userID int, targetID int, targetType string) error {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return fmt.Errorf("logger not found in context")
	}

	result, err := s.db.Exec(ctx, deleteLikeSQL, userID, targetID, targetType)
	if err != nil {
		logger.Printf("error deleting like: %v", err)
		return fmt.Errorf("deleting like: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("like not found")
	}

	return nil
}

func (s *Service) GetFeedItemLikes(ctx context.Context, feedItemID int, cursor string, pageSize int) (*PaginatedLikes, error) {
	return s.getLikes(ctx, getFeedItemLikesSQL, feedItemID, TargetTypeFeedItem, cursor, pageSize)
}

func (s *Service) GetUserCarLikes(ctx context.Context, userCarID int, cursor string, pageSize int) (*PaginatedLikes, error) {
	return s.getLikes(ctx, getUserCarLikesSQL, userCarID, TargetTypeUserCar, cursor, pageSize)
}

func (s *Service) GetUserReceivedLikes(ctx context.Context, userID int, cursor string, pageSize int) (*PaginatedLikes, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return nil, fmt.Errorf("logger not found in context")
	}

	if pageSize <= 0 {
		pageSize = 10
	}

	var cursorTime *time.Time
	var cursorID int
	if cursor != "" {
		decoded, err := common.DecodeCursor(cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		cursorTime = &decoded.Timestamp
		cursorID = decoded.ID
	}

	rows, err := s.db.Query(ctx, getUserReceivedLikesSQL, userID, cursorTime, cursorID, pageSize+1)
	if err != nil {
		logger.Printf("error querying received likes: %v", err)
		return nil, fmt.Errorf("querying received likes: %w", err)
	}
	defer rows.Close()

	likes := make([]Like, 0, pageSize+1)
	for rows.Next() {
		var like Like
		var createdAt time.Time
		if err := rows.Scan(&like.ID, &like.UserID, &like.TargetID, &like.TargetType, &createdAt); err != nil {
			logger.Printf("error scanning like: %v", err)
			return nil, fmt.Errorf("scanning like: %w", err)
		}
		like.CreatedAt = common.FormatTimestamp(createdAt)
		likes = append(likes, like)
	}

	result := &PaginatedLikes{
		Items: likes,
	}

	if len(likes) > pageSize {
		lastItem := likes[pageSize-1]
		timestamp, _ := common.ParseTimestamp(lastItem.CreatedAt)
		result.NextCursor = common.EncodeCursor(timestamp, lastItem.ID)
		result.Items = likes[:pageSize]
	}

	return result, nil
}

func (s *Service) getLikes(ctx context.Context, query string, targetID int, targetType string, cursor string, pageSize int) (*PaginatedLikes, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return nil, fmt.Errorf("logger not found in context")
	}

	if pageSize <= 0 {
		pageSize = 10
	}

	var cursorTime *time.Time
	var cursorID int
	if cursor != "" {
		decoded, err := common.DecodeCursor(cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		cursorTime = &decoded.Timestamp
		cursorID = decoded.ID
	}

	rows, err := s.db.Query(ctx, query, targetID, targetType, cursorTime, cursorID, pageSize+1)
	if err != nil {
		logger.Printf("error querying likes: %v", err)
		return nil, fmt.Errorf("querying likes: %w", err)
	}
	defer rows.Close()

	likes := make([]Like, 0, pageSize+1)
	for rows.Next() {
		var like Like
		var createdAt time.Time
		if err := rows.Scan(&like.ID, &like.UserID, &like.TargetID, &like.TargetType, &createdAt); err != nil {
			logger.Printf("error scanning like: %v", err)
			return nil, fmt.Errorf("scanning like: %w", err)
		}
		like.CreatedAt = common.FormatTimestamp(createdAt)
		likes = append(likes, like)
	}

	result := &PaginatedLikes{
		Items: likes,
	}

	if len(likes) > pageSize {
		lastItem := likes[pageSize-1]
		timestamp, _ := common.ParseTimestamp(lastItem.CreatedAt)
		result.NextCursor = common.EncodeCursor(timestamp, lastItem.ID)
		result.Items = likes[:pageSize]
	}

	return result, nil
}

func (s *Service) UserHasLiked(ctx context.Context, userID int, targetID int, targetType string) (bool, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return false, fmt.Errorf("logger not found in context")
	}

	var hasLiked bool
	if err := s.db.QueryRow(ctx, checkUserLikedSQL, userID, targetID, targetType).Scan(&hasLiked); err != nil {
		logger.Printf("error checking if user has liked: %v", err)
		return false, fmt.Errorf("checking if user has liked: %w", err)
	}

	return hasLiked, nil
}

func (s *Service) GetLikesCount(ctx context.Context, targetID int, targetType string) (int, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return 0, fmt.Errorf("logger not found in context")
	}

	var count int
	if err := s.db.QueryRow(ctx, getLikesCountSQL, targetID, targetType).Scan(&count); err != nil {
		logger.Printf("error getting likes count: %v", err)
		return 0, fmt.Errorf("getting likes count: %w", err)
	}

	return count, nil
}
