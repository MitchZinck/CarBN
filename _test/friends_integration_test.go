package test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFriendsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("FriendRequestFlow", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create two test users
		user1 := createTestUser(t)
		user2 := createTestUser(t)
		user1ID := createTestUserInDB(t, user1)
		user2ID := createTestUserInDB(t, user2)

		// Login users
		user1Token := loginUser(t, user1.Email, user1.Password)
		user2Token := loginUser(t, user2.Email, user2.Password)

		// Send friend request
		reqPayload := map[string]interface{}{
			"friend_id": user2ID,
		}
		resp, _ := makeRequest(t, http.MethodPost, "/friends/request", reqPayload, user1Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify request in database
		var requestID int
		var status string
		err := testDB.QueryRow(subCtx, `
			SELECT id, status
			FROM friends
			WHERE user_id = $1 AND friend_id = $2
		`, user1ID, user2ID).Scan(&requestID, &status)
		require.NoError(t, err)
		assert.Equal(t, "pending", status)

		// Accept friend request
		acceptPayload := map[string]interface{}{
			"request_id": requestID,
			"response":   "accept",
		}
		resp, _ = makeRequest(t, http.MethodPost, "/friends/respond", acceptPayload, user2Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify friendship status
		err = testDB.QueryRow(subCtx, `
			SELECT status
			FROM friends
			WHERE id = $1
		`, requestID).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "accepted", status)

		// Verify feed entries with proper context
		var feedCount int
		err = testDB.QueryRow(subCtx, `
			SELECT COUNT(*)
			FROM feed
			WHERE user_id IN ($1, $2)
			AND type = 'friend_accepted'
		`, user1ID, user2ID).Scan(&feedCount)
		require.NoError(t, err)
		assert.Equal(t, 2, feedCount, "Should have feed entries for both users")
	})

	t.Run("RejectFriendRequestFlow", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create two test users
		user1 := createTestUser(t)
		user2 := createTestUser(t)
		user1ID := createTestUserInDB(t, user1)
		user2ID := createTestUserInDB(t, user2)

		// Login users
		user1Token := loginUser(t, user1.Email, user1.Password)
		user2Token := loginUser(t, user2.Email, user2.Password)

		// Send friend request
		reqPayload := map[string]interface{}{
			"friend_id": user2ID,
		}
		resp, _ := makeRequest(t, http.MethodPost, "/friends/request", reqPayload, user1Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get request ID
		var requestID int
		err := testDB.QueryRow(subCtx, `
			SELECT id
			FROM friends
			WHERE user_id = $1 AND friend_id = $2
		`, user1ID, user2ID).Scan(&requestID)
		require.NoError(t, err)

		// Reject friend request
		rejectPayload := map[string]interface{}{
			"request_id": requestID,
			"response":   "reject",
		}
		resp, _ = makeRequest(t, http.MethodPost, "/friends/respond", rejectPayload, user2Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify rejection status
		var status string
		err = testDB.QueryRow(subCtx, `
			SELECT status
			FROM friends
			WHERE id = $1
		`, requestID).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "rejected", status)
	})
}
