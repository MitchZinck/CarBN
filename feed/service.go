package feed

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

type FeedItem struct {
	ID            int    `json:"id"`
	Type          string `json:"type"`
	ReferenceID   int    `json:"reference_id"`
	CreatedAt     string `json:"created_at"`
	UserID        int    `json:"user_id"`
	RelatedUserId *int   `json:"related_user_id"`
	LikeCount     int    `json:"like_count"`
	UserLiked     bool   `json:"user_liked"`
}

type PaginatedFeed struct {
	Items      []FeedItem `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// SQL queries as constants for better maintainability
const (
	getFeedSQL = `
		WITH friend_ids AS (
			SELECT user_id AS id
			FROM friends 
			WHERE friend_id = $1 AND status = 'accepted'
			UNION
			SELECT friend_id AS id
			FROM friends 
			WHERE user_id = $1 AND status = 'accepted'
		)
		SELECT f.id, f.type, f.reference_id, f.created_at, f.user_id, f.related_user_id,
			   COUNT(l.id) as like_count,
			   EXISTS(SELECT 1 FROM likes WHERE target_id = f.id AND target_type = 'feed_item' AND user_id = $1) as user_liked
		FROM feed f
		LEFT JOIN likes l ON f.id = l.target_id
		WHERE (
			-- Include user's own feed items
			f.user_id = $1
			-- Or feed items where either the user or related user is a friend
			OR (
				f.user_id IN (SELECT id FROM friend_ids)
				OR f.related_user_id IN (SELECT id FROM friend_ids)
			)
		)
		AND ($2::TIMESTAMPTZ IS NULL OR (f.created_at, f.id) < ($2::TIMESTAMPTZ, $3::INT))
		GROUP BY f.id, f.type, f.reference_id, f.created_at, f.user_id, f.related_user_id
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT $4`

	getGlobalFeedSQL = `
		SELECT f.id, f.type, f.reference_id, f.created_at, f.user_id, f.related_user_id,
			   COUNT(l.id) as like_count,
			   EXISTS(SELECT 1 FROM likes WHERE target_id = f.id AND target_type = 'feed_item' AND user_id = $1) as user_liked
		FROM feed f
		LEFT JOIN likes l ON f.id = l.target_id
		WHERE ($2::TIMESTAMPTZ IS NULL OR (f.created_at, f.id) < ($2::TIMESTAMPTZ, $3::INT))
		GROUP BY f.id, f.type, f.reference_id, f.created_at, f.user_id, f.related_user_id
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT $4`

	createFeedSQL = `
		INSERT INTO feed (user_id, type, reference_id, related_user_id)
		VALUES ($1, $2, $3, $4)`

	getFeedItemSQL = `
		SELECT f.id, f.type, f.reference_id, f.created_at, f.user_id, f.related_user_id,
			   COUNT(l.id) as like_count,
			   EXISTS(SELECT 1 FROM likes WHERE target_id = f.id AND target_type = 'feed_item' AND user_id = $2) as user_liked
		FROM feed f
		LEFT JOIN likes l ON f.id = l.target_id
		WHERE f.id = $1
		GROUP BY f.id, f.type, f.reference_id, f.created_at, f.user_id, f.related_user_id`
)

// GetFeed retrieves feed items from user's friends, or globally if feedType is "global"
func (s *Service) GetFeed(ctx context.Context, userID int, cursor string, pageSize int, feedType string) (*PaginatedFeed, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return nil, fmt.Errorf("logger not found in context")
	}

	if pageSize <= 0 {
		pageSize = 10
		logger.Printf("using default pageSize value: %d", pageSize)
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

	var query string
	if feedType == "global" {
		query = getGlobalFeedSQL
	} else {
		query = getFeedSQL
	}

	feedRows, err := s.db.Query(ctx, query, userID, cursorTime, cursorID, pageSize+1)
	if err != nil {
		logger.Printf("error querying feed: %v", err)
		return nil, fmt.Errorf("querying feed: %w", err)
	}
	defer feedRows.Close()

	feed := make([]FeedItem, 0, pageSize+1)
	for feedRows.Next() {
		var item FeedItem
		var createdAt time.Time
		if err := feedRows.Scan(
			&item.ID,
			&item.Type,
			&item.ReferenceID,
			&createdAt,
			&item.UserID,
			&item.RelatedUserId,
			&item.LikeCount,
			&item.UserLiked,
		); err != nil {
			logger.Printf("error scanning feed item: %v", err)
			return nil, fmt.Errorf("scanning feed item: %w", err)
		}
		item.CreatedAt = common.FormatTimestamp(createdAt)
		feed = append(feed, item)
	}

	result := &PaginatedFeed{
		Items: feed,
	}

	// If we got more items than requested, set the next cursor
	if len(feed) > pageSize {
		lastItem := feed[pageSize-1]
		timestamp, _ := common.ParseTimestamp(lastItem.CreatedAt)
		result.NextCursor = common.EncodeCursor(timestamp, lastItem.ID)
		result.Items = feed[:pageSize]
	}

	logger.Printf("successfully retrieved %d feed items", len(result.Items))
	return result, nil
}

// CreateFeed adds a new feed item
func (s *Service) CreateFeed(ctx context.Context, userID int, feedType string, referenceID int, relatedUserID int) error {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return fmt.Errorf("logger not found in context")
	}

	var relatedUserIDPtr *int
	if relatedUserID > 0 {
		relatedUserIDPtr = &relatedUserID
	}

	if _, err := s.db.Exec(ctx, createFeedSQL, userID, feedType, referenceID, relatedUserIDPtr); err != nil {
		logger.Printf("error creating feed item: %v", err)
		return fmt.Errorf("creating feed item: %w", err)
	}

	logger.Printf("created feed item for user %d of type %s with reference %d", userID, feedType, referenceID)
	return nil
}

// GetFeedItem retrieves a single feed item by ID
func (s *Service) GetFeedItem(ctx context.Context, feedItemID int, currentUserID int) (*FeedItem, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		return nil, fmt.Errorf("logger not found in context")
	}

	var item FeedItem
	var createdAt time.Time
	err := s.db.QueryRow(ctx, getFeedItemSQL, feedItemID, currentUserID).Scan(
		&item.ID,
		&item.Type,
		&item.ReferenceID,
		&createdAt,
		&item.UserID,
		&item.RelatedUserId,
		&item.LikeCount,
		&item.UserLiked,
	)
	if err != nil {
		logger.Printf("error getting feed item: %v", err)
		return nil, fmt.Errorf("getting feed item: %w", err)
	}

	item.CreatedAt = common.FormatTimestamp(createdAt)
	return &item, nil
}
