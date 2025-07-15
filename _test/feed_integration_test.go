package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FriendRequest struct {
	FriendID  int `json:"friend_id"`
	RequestID int `json:"request_id"`
}

type FeedItem struct {
	Type        string `json:"type"`
	ReferenceID int    `json:"reference_id"`
	UserID      int    `json:"user_id"`
}

const (
	FeedTypeScan  = "scan"
	FeedTypeTrade = "trade"
)

func TestFeedIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("GetFeedWithFriends", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create test users
		user1 := createTestUser(t)
		user2 := createTestUser(t)
		user3 := createTestUser(t)

		user1ID := createTestUserInDB(t, user1)
		user2ID := createTestUserInDB(t, user2)
		user3ID := createTestUserInDB(t, user3)

		// Create friendships in database with context
		_, err := testDB.Exec(subCtx, `
			INSERT INTO friends (user_id, friend_id, status)
			VALUES ($1, $2, 'accepted'), ($1, $3, 'accepted')
		`, user1ID, user2ID, user3ID)
		require.NoError(t, err)

		// Create feed items for all users
		_, err = testDB.Exec(subCtx, `
			INSERT INTO feed (user_id, type, reference_id)
			VALUES
				($1, 'car_scanned', 1),
				($2, 'car_scanned', 2),
				($3, 'car_scanned', 3),
				($1, 'trade_completed', 1),
				($2, 'trade_completed', 2)
		`, user1ID, user2ID, user3ID)
		require.NoError(t, err)

		// Login user1
		token := loginUser(t, user1.Email, user1.Password)

		// Test feed retrieval with different limits
		testCases := []struct {
			maxItems    int
			expectCount int
		}{
			{2, 2},
			{3, 3},
			{5, 5},
		}

		for _, tc := range testCases {
			resp, body := makeRequest(t, http.MethodGet,
				fmt.Sprintf("/feed?max_items=%d", tc.maxItems),
				nil, token)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var feed []struct {
				ID          int    `json:"id"`
				Type        string `json:"type"`
				ReferenceID int    `json:"reference_id"`
			}
			require.NoError(t, json.Unmarshal(body, &feed))
			assert.Len(t, feed, tc.expectCount)
		}

		// Verify feed items are properly ordered
		resp, body := makeRequest(t, http.MethodGet, "/feed?max_items=10", nil, token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var feed []struct {
			ID          int    `json:"id"`
			Type        string `json:"type"`
			ReferenceID int    `json:"reference_id"`
		}
		require.NoError(t, json.Unmarshal(body, &feed))

		// Verify items are in reverse chronological order
		for i := 1; i < len(feed); i++ {
			assert.True(t, feed[i-1].ID > feed[i].ID,
				"Feed items should be in reverse chronological order")
		}
	})

	t.Run("GetFeedWithoutFriends", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create isolated user
		user := createTestUser(t)
		userID := createTestUserInDB(t, user)

		// Create feed items for user
		_, err := testDB.Exec(subCtx, `
			INSERT INTO feed (user_id, type, reference_id)
			VALUES
				($1, 'car_scanned', 1),
				($1, 'trade_completed', 1)
		`, userID)
		require.NoError(t, err)

		// Login and get feed
		token := loginUser(t, user.Email, user.Password)
		resp, body := makeRequest(t, http.MethodGet, "/feed", nil, token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var feed []struct {
			ID          int    `json:"id"`
			Type        string `json:"type"`
			ReferenceID int    `json:"reference_id"`
		}
		require.NoError(t, json.Unmarshal(body, &feed))

		// Should only see own feed items
		assert.Len(t, feed, 2)
		for _, item := range feed {
			var itemUserID int
			err := testDB.QueryRow(subCtx,
				"SELECT user_id FROM feed WHERE id = $1",
				item.ID).Scan(&itemUserID)
			require.NoError(t, err)
			assert.Equal(t, userID, itemUserID)
		}
	})
}

func TestLikesIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("LikeFeedItem", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create test users
		user1 := createTestUser(t)
		user2 := createTestUser(t)

		user1ID := createTestUserInDB(t, user1)
		user2ID := createTestUserInDB(t, user2)

		// Create a feed item
		var feedItemID int
		err := testDB.QueryRow(subCtx, `
			INSERT INTO feed (user_id, type, reference_id)
			VALUES ($1, 'car_scanned', 1)
			RETURNING id
		`, user1ID).Scan(&feedItemID)
		require.NoError(t, err)

		// Login user2
		token := loginUser(t, user2.Email, user2.Password)

		// Like the feed item
		resp, body := makeRequest(t, http.MethodPost,
			fmt.Sprintf("/likes/%d", feedItemID),
			nil, token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var like struct {
			ID         int    `json:"id"`
			UserID     int    `json:"user_id"`
			FeedItemID int    `json:"feed_item_id"`
			CreatedAt  string `json:"created_at"`
		}
		require.NoError(t, json.Unmarshal(body, &like))
		assert.Equal(t, user2ID, like.UserID)
		assert.Equal(t, feedItemID, like.FeedItemID)

		// Unlike the feed item
		resp, _ = makeRequest(t, http.MethodDelete,
			fmt.Sprintf("/likes/%d", feedItemID),
			nil, token)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)

		// Verify like was deleted
		var count int
		err = testDB.QueryRow(subCtx, `
			SELECT COUNT(*) FROM likes
			WHERE user_id = $1 AND feed_item_id = $2
		`, user2ID, feedItemID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("GetFeedItemLikes", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create test user and feed item
		user := createTestUser(t)
		userID := createTestUserInDB(t, user)

		var feedItemID int
		err := testDB.QueryRow(subCtx, `
			INSERT INTO feed (user_id, type, reference_id)
			VALUES ($1, 'car_scanned', 1)
			RETURNING id
		`, userID).Scan(&feedItemID)
		require.NoError(t, err)

		// Create multiple likes
		_, err = testDB.Exec(subCtx, `
			INSERT INTO likes (user_id, feed_item_id)
			VALUES ($1, $2), ($1, $2), ($1, $2)
		`, userID, feedItemID)
		require.NoError(t, err)

		// Login and get likes
		token := loginUser(t, user.Email, user.Password)
		resp, body := makeRequest(t, http.MethodGet,
			fmt.Sprintf("/likes/feed-item/%d", feedItemID),
			nil, token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			Items []struct {
				ID         int    `json:"id"`
				UserID     int    `json:"user_id"`
				FeedItemID int    `json:"feed_item_id"`
				CreatedAt  string `json:"created_at"`
			} `json:"items"`
			NextCursor string `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Len(t, result.Items, 3)
		for _, item := range result.Items {
			assert.Equal(t, userID, item.UserID)
			assert.Equal(t, feedItemID, item.FeedItemID)
		}
	})
}
