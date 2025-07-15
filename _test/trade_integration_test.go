package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTradeIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create test users
	user1 := createTestUser(t)
	user2 := createTestUser(t)
	user1ID := createTestUserInDB(t, user1)
	user2ID := createTestUserInDB(t, user2)

	// Helper to log a user in and return the access token
	loginUser := func(t *testing.T, email, password string) string {
		resp, body := makeRequest(t, http.MethodPost, "/login",
			map[string]string{"email": email, "password": password}, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var authResp struct {
			AccessToken string `json:"access_token"`
		}
		require.NoError(t, json.Unmarshal(body, &authResp))
		return authResp.AccessToken
	}

	t.Run("AcceptTradeScenario", func(t *testing.T) {
		// Create test context
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Login users
		user1Token := loginUser(t, user1.Email, user1.Password)
		user2Token := loginUser(t, user2.Email, user2.Password)

		// Insert multiple cars for both users
		_, err := testDB.Exec(subCtx, `
			INSERT INTO cars (make, model, year	)
			VALUES
				('Subaru', 'Outback', 2022),
				('Mazda', '3', 2021),
				('BMW', 'i3', 2023),
				('Jeep', 'Wrangler', 2022),
				('Hyundai', 'Elantra', 2020);
		`)
		require.NoError(t, err)

		// Assign three cars to user1, two to user2
		_, err = testDB.Exec(subCtx, `
			INSERT INTO user_cars (user_id, car_id, color)
			VALUES
				($1, (SELECT id FROM cars WHERE make = 'Subaru' LIMIT 1), 'White'),
				($1, (SELECT id FROM cars WHERE make = 'Mazda' LIMIT 1), 'Blue'),
				($1, (SELECT id FROM cars WHERE make = 'BMW' LIMIT 1), 'Silver'),
				($2, (SELECT id FROM cars WHERE make = 'Jeep' LIMIT 1), 'Black'),
				($2, (SELECT id FROM cars WHERE make = 'Hyundai' LIMIT 1), 'Red');
		`, user1ID, user2ID)
		require.NoError(t, err)

		// Fetch user_car IDs for user1
		var carSubaruID, carMazdaID, carBMWID int
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT id FROM user_cars 
				WHERE user_id = $1 AND car_id = (SELECT id FROM cars WHERE make = 'Subaru' LIMIT 1)
			`, user1ID).Scan(&carSubaruID),
		)
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT id FROM user_cars
				WHERE user_id = $1 AND car_id = (SELECT id FROM cars WHERE make = 'Mazda' LIMIT 1)
			`, user1ID).Scan(&carMazdaID),
		)
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT id FROM user_cars
				WHERE user_id = $1 AND car_id = (SELECT id FROM cars WHERE make = 'BMW' LIMIT 1)
			`, user1ID).Scan(&carBMWID),
		)

		// Fetch user_car IDs for user2
		var carJeepID, carHyundaiID int
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT id FROM user_cars
				WHERE user_id = $1 AND car_id = (SELECT id FROM cars WHERE make = 'Jeep' LIMIT 1)
			`, user2ID).Scan(&carJeepID),
		)
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT id FROM user_cars
				WHERE user_id = $1 AND car_id = (SELECT id FROM cars WHERE make = 'Hyundai' LIMIT 1)
			`, user2ID).Scan(&carHyundaiID),
		)

		// Create trade request
		createTradePayload := map[string]interface{}{
			"user_id_to":             user2ID,
			"user_from_user_car_ids": []int{carSubaruID, carMazdaID, carBMWID},
			"user_to_user_car_ids":   []int{carJeepID, carHyundaiID},
		}
		resp, _ := makeRequest(t, http.MethodPost, "/trade/request", createTradePayload, user1Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get the trade ID
		var tradeID int
		err = testDB.QueryRow(subCtx,
			"SELECT id FROM trades WHERE user_id_from = $1 AND user_id_to = $2",
			user1ID, user2ID).Scan(&tradeID)
		require.NoError(t, err)

		// User2 logs in and accepts
		respondPayload := map[string]interface{}{
			"trade_id": tradeID,
			"response": "accept",
		}
		resp, _ = makeRequest(t, http.MethodPost, "/trade/respond", respondPayload, user2Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify trade status
		var status string
		err = testDB.QueryRow(subCtx,
			"SELECT status FROM trades WHERE id = $1", tradeID).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "accepted", status)

		// Verify car ownership changed
		var newOwnerID int
		err = testDB.QueryRow(subCtx,
			"SELECT user_id FROM user_cars WHERE id = $1", carSubaruID).Scan(&newOwnerID)
		require.NoError(t, err)
		assert.Equal(t, user2ID, newOwnerID)
	})

	t.Run("DeclineTradeScenario", func(t *testing.T) {
		// Create test context
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Login users
		user1Token := loginUser(t, user1.Email, user1.Password)
		user2Token := loginUser(t, user2.Email, user2.Password)

		// Insert more cars
		_, err := testDB.Exec(subCtx, `
			INSERT INTO cars (make, model, year)
			VALUES
				('Toyota', 'Camry', 2023),
				('Chevy', 'Bolt', 2022),
				('Nissan', 'Sentra', 2021),
				('Tesla', 'Model 3', 2023),
				('Kia', 'Rio', 2020);
		`)
		require.NoError(t, err)

		// Assign cars to each user
		_, err = testDB.Exec(subCtx, `
			INSERT INTO user_cars (user_id, car_id, color)
			VALUES
				($1, (SELECT id FROM cars WHERE make = 'Toyota' LIMIT 1), 'Black'),
				($1, (SELECT id FROM cars WHERE make = 'Nissan' LIMIT 1), 'Grey'),
				($1, (SELECT id FROM cars WHERE make = 'Kia' LIMIT 1), 'Green'),
				($2, (SELECT id FROM cars WHERE make = 'Chevy' LIMIT 1), 'Silver'),
				($2, (SELECT id FROM cars WHERE make = 'Tesla' LIMIT 1), 'Red');
		`, user1ID, user2ID)
		require.NoError(t, err)

		// Fetch the user_car IDs specifically for Toyota (user1) and Chevy (user2)
		var userCarToyotaID, userCarChevyID int
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT uc.id 
				FROM user_cars uc
				JOIN cars c ON uc.car_id = c.id
				WHERE uc.user_id = $1 AND c.make = 'Toyota' LIMIT 1
			`, user1ID).Scan(&userCarToyotaID),
		)
		require.NoError(t,
			testDB.QueryRow(subCtx, `
				SELECT uc.id 
				FROM user_cars uc
				JOIN cars c ON uc.car_id = c.id
				WHERE uc.user_id = $1 AND c.make = 'Chevy' LIMIT 1
			`, user2ID).Scan(&userCarChevyID),
		)

		// Create the trade request
		createTradePayload := map[string]interface{}{
			"user_id_to":             user2ID,
			"user_from_user_car_ids": []int{userCarToyotaID},
			"user_to_user_car_ids":   []int{userCarChevyID},
		}
		resp, _ := makeRequest(t, http.MethodPost, "/trade/request", createTradePayload, user1Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get the trade ID
		var tradeID int
		err = testDB.QueryRow(subCtx,
			"SELECT id FROM trades WHERE user_id_from = $1 AND user_id_to = $2 AND status = 'pending'",
			user1ID, user2ID).Scan(&tradeID)
		require.NoError(t, err)

		// User2 logs in and declines
		respondPayload := map[string]interface{}{
			"trade_id": tradeID,
			"response": "decline",
		}
		resp, _ = makeRequest(t, http.MethodPost, "/trade/respond", respondPayload, user2Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify trade status
		var status string
		err = testDB.QueryRow(subCtx,
			"SELECT status FROM trades WHERE id = $1", tradeID).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "declined", status)

		// Confirm car ownership did not change
		var ownerToyota, ownerChevy int
		require.NoError(t,
			testDB.QueryRow(subCtx,
				"SELECT user_id FROM user_cars WHERE id = $1", userCarToyotaID,
			).Scan(&ownerToyota),
		)
		assert.Equal(t, user1ID, ownerToyota)

		require.NoError(t,
			testDB.QueryRow(subCtx,
				"SELECT user_id FROM user_cars WHERE id = $1", userCarChevyID,
			).Scan(&ownerChevy),
		)
		assert.Equal(t, user2ID, ownerChevy)
	})
}
