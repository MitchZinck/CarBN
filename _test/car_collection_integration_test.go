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

func TestCarCollectionIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create test user
	user := createTestUser(t)
	createTestUserInDB(t, user)

	// Insert dummy cars with context
	_, err := testDB.Exec(ctx, `
		INSERT INTO cars (
			make, model, year, trim, horsepower, torque, top_speed,
			acceleration, engine_type, drivetrain_type, curb_weight,
			price, description, rarity
		)
		VALUES
			('Toyota', 'Corolla', 2020, 'XSE', 169, 151, 120, 7.1, 
			'2.0L 4-Cylinder', 'FWD', 3150.5, 25000, 
			'Modern compact sedan with sporty features', 1),
			('Honda', 'Civic', 2021, 'Sport', 180, 177, 137, 6.7,
			'1.5L Turbo 4-Cylinder', 'FWD', 2877.9, 23000,
			'Athletic and fuel-efficient compact car', 1)
	`)
	require.NoError(t, err)

	// Link user to cars with context and image paths
	_, err = testDB.Exec(ctx, `
		INSERT INTO user_cars (user_id, car_id, color, low_res_image, high_res_image)
		SELECT u.id, c.id, 'Red', 
			format('car_%s/red/low_res.jpg', c.id),
			format('car_%s/red/high_res.jpg', c.id)
		FROM users u, cars c
		WHERE u.email = $1
		  AND (c.make = 'Toyota' OR c.make = 'Honda')
	`, user.Email)
	require.NoError(t, err)

	// Login to get access token
	resp, body := makeRequest(t, http.MethodPost, "/login",
		map[string]string{
			"email":    user.Email,
			"password": user.Password,
		}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var authResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.Unmarshal(body, &authResp))

	// Get user details
	resp, body = makeRequest(t, http.MethodGet, "/user/details", nil, authResp.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var userDetailsResp struct {
		ID             int    `json:"id"`
		Email          string `json:"email"`
		DisplayName    string `json:"display_name"`
		FollowerCount  int    `json:"followers_count"`
		ProfilePicture string `json:"profile_picture"`
	}
	require.NoError(t, json.Unmarshal(body, &userDetailsResp))

	t.Run("Test Get Car Collection", func(t *testing.T) {
		// Get user's cars with pagination
		url := fmt.Sprintf("/user/%d/cars?limit=1&offset=0", userDetailsResp.ID)
		resp, body = makeRequest(t, http.MethodGet, url, nil, authResp.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

		var cars []car
		require.NoError(t, json.Unmarshal(body, &cars))
		assert.Len(t, cars, 1, "Expected to retrieve 1 car due to limit")

		// Get second page
		url = fmt.Sprintf("/user/%d/cars?limit=1&offset=1", userDetailsResp.ID)
		resp, body = makeRequest(t, http.MethodGet, url, nil, authResp.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

		require.NoError(t, json.Unmarshal(body, &cars))
		assert.Len(t, cars, 1, "Expected to retrieve 1 car from second page")
	})

	t.Run("Test Get Specific Cars", func(t *testing.T) {
		// First get all cars to get their user_car_ids
		url := fmt.Sprintf("/user/%d/cars", userDetailsResp.ID)
		resp, body = makeRequest(t, http.MethodGet, url, nil, authResp.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

		var allCars []car
		require.NoError(t, json.Unmarshal(body, &allCars))
		require.GreaterOrEqual(t, len(allCars), 2, "Expected at least 2 cars")

		// Now get specific cars using their user_car_ids
		userCarIDs := []int{allCars[0].UserCarID, allCars[1].UserCarID}
		url = fmt.Sprintf("/user/cars?ids=%d,%d", userCarIDs[0], userCarIDs[1])
		resp, body = makeRequest(t, http.MethodGet, url, nil, authResp.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

		var specificCars []car
		require.NoError(t, json.Unmarshal(body, &specificCars))
		assert.Len(t, specificCars, 2, "Expected to retrieve both specific cars")

		// Verify we got the correct cars
		assert.Equal(t, userCarIDs[0], specificCars[0].UserCarID)
		assert.Equal(t, userCarIDs[1], specificCars[1].UserCarID)
	})
}

type car struct {
	ID             int      `json:"id"`
	UserCarID      int      `json:"user_car_id"`
	Make           string   `json:"make"`
	Model          string   `json:"model"`
	Year           int      `json:"year"`
	Color          string   `json:"color"`
	Trim           string   `json:"trim,omitempty"`
	Horsepower     *int     `json:"horsepower,omitempty"`
	Torque         *int     `json:"torque,omitempty"`
	TopSpeed       *int     `json:"top_speed,omitempty"`
	Acceleration   *float64 `json:"acceleration,omitempty"`
	EngineType     *string  `json:"engine_type,omitempty"`
	DrivetrainType *string  `json:"drivetrain_type,omitempty"`
	CurbWeight     *float64 `json:"curb_weight,omitempty"`
	Price          *int     `json:"price,omitempty"`
	Description    *string  `json:"description,omitempty"`
	Rarity         *int     `json:"rarity,omitempty"`
	LowResImage    *string  `json:"low_res_image,omitempty"`
	HighResImage   *string  `json:"high_res_image,omitempty"`
	DateCollected  *string  `json:"date_collected,omitempty"`
}
