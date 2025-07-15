# User API Documentation

## Endpoints

### Get User Profile
- **URL**: `/user/{user_id}/details`
- **Method**: `GET`
- **Authentication**: Required
- **Parameters**:
  - `user_id`: ID of the user to fetch (path parameter)
- **Response**:
  - Success (200 OK):
  ```json
  {
    "id": 123,
    "display_name": "username",
    "followers_count": 42,
    "friend_count": 25,
    "profile_picture": "images/profile_pictures/user_123.jpg",
    "is_friend": true,
    "is_private": false,
    "car_count": 10,
    "email": "user@email.com"
  }
  ```
  - Email is only returned if the request is for the logged in user details
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): User not found
  - Error (500 Internal Server Error): Server-side error
  - `friend_count` represents the total number of accepted friend connections

### Get Car Collection
- **URL**: `/user/{user_id}/cars`
- **Method**: `GET`
- **Authentication**: Required
- **Parameters**:
  - `user_id`: ID of the user whose collection to fetch (path parameter)
  - `limit`: Maximum number of cars to return (query parameter, default: 10)
  - `offset`: Number of cars to skip (query parameter, default: 0)
- **Response**:
  - Success (200 OK):
  ```json
  [
    {
      "id": 1,
      "user_car_id": 101,
      "make": "Toyota",
      "model": "Corolla",
      "year": 2020,
      "color": "Red",
      "trim": "XSE",
      "horsepower": 169,
      "torque": 151,
      "top_speed": 120,
      "acceleration": 7.1,
      "engine_type": "2.0L 4-Cylinder",
      "drivetrain_type": "FWD",
      "curb_weight": 3150.5,
      "price": 25000,
      "description": "Modern compact sedan with sporty features",
      "rarity": 1,
      "low_res_image": "car_1/red/low_res.jpg",
      "high_res_image": "car_1/red/high_res.jpg",
      "date_collected": "2024-01-20T15:30:00.000Z",
      "likes_count": 42,
      "upgrades": [
        {
          "id": 1,
          "upgrade_type": "premium_image",
          "active": true,
          "metadata": {
            "premium_low_res": "car_1/red/premium_low_res.jpg",
            "premium_high_res": "car_1/red/premium_high_res.jpg"
          },
          "created_at": "2024-01-20T15:30:00.000Z",
          "updated_at": "2024-01-20T15:30:00.000Z"
        }
      ]
    }
  ]
  ```
  - Error (400 Bad Request): Invalid user ID format
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): No cars found
  - Error (500 Internal Server Error): Server-side error

### Get Specific User Cars
- **URL**: `/user/cars`
- **Method**: `GET`
- **Authentication**: Required
- **Parameters**:
  - `ids`: Comma-separated list of user car IDs to fetch (query parameter, required)
- **Response**:
  - Success (200 OK): Same format as Get Car Collection
  - Error (400 Bad Request): Missing or invalid car IDs format
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): No cars found
  - Error (500 Internal Server Error): Server-side error

### Get Pending Friend Requests
- **URL**: `/user/friend-requests`
- **Method**: `GET`
- **Authentication**: Required
- **Response**:
  - Success (200 OK):
  ```json
  [
    {
      "id": 1,
      "user_id": 123,
      "user_display_name": "sender",
      "user_profile_picture": "images/profile_pictures/user_452.jpg",
      "friend_id": 456,
      "friend_display_name": "recipient",
      "friend_profile_picture": "images/profile_pictures/user_456.jpg"
    }
  ]
  ```
  - Error (401 Unauthorized): Invalid or missing token
  - Error (500 Internal Server Error): Server-side error

### Upload Profile Picture
- **URL**: `/user/profile/picture`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
  ```json
  {
    "base64_image": "data:image/jpeg;base64,..."
  }
  ```
- **Requirements**:
  - Image must be 512x512 pixels
  - Image must be in a valid format (JPEG)
- **Response**:
  - Success (200 OK): Profile picture updated successfully
  - Error (400 Bad Request): Invalid image format or dimensions
  - Error (401 Unauthorized): Invalid or missing token
  - Error (500 Internal Server Error): Failed to update profile picture

### Search Users
- **URL**: `/user/search`
- **Method**: `GET`
- **Authentication**: Required
- **Parameters**:
  - `q`: Search query for display name (query parameter)
- **Response**:
  - Success (200 OK):
  ```json
  [
    {
      "id": 123,
      "display_name": "username",
      "followers_count": 42,
      "profile_picture": "images/profile_pictures/user_123.jpg"
    }
  ]
  ```
  - Error (400 Bad Request): Missing search query
  - Error (401 Unauthorized): Invalid or missing token
  - Error (500 Internal Server Error): Failed to search users

### Sell Car
- **URL**: `/user/cars/{user_car_id}/sell`
- **Method**: `POST`
- **Authentication**: Required
- **URL Parameters**:
  - `user_car_id`: ID of the car to sell
- **Response**:
  - Success (200 OK):
  ```json
  {
    "message": "car sold successfully",
    "currency_earned": 500
  }
  ```
  - Error (400 Bad Request): Invalid car ID format
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): Car not found or not owned by user
  - Error (500 Internal Server Error): Server error

The amount of currency earned is based on the car's rarity:
- Rarity 1: 500 currency
- Rarity 2: 750 currency 
- Rarity 3: 1250 currency
- Rarity 4: 2000 currency
- Rarity 5: 5000 currency

Note: When a car is sold, it remains in the system's history (user_id = 0) for reference in trade history and feed items.

### Get Car Upgrades
- **URL**: `/user/cars/{user_car_id}/upgrades`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `user_car_id`: ID of the car to check upgrades for
- **Response**:
  - Success (200 OK):
  ```json
  [
    {
      "id": 1,
      "upgrade_type": "premium_image",
      "active": true,
      "metadata": {
        "original_low_res": "car_1/red/low_res.jpg",
        "original_high_res": "car_1/red/high_res.jpg"
      },
      "created_at": "2024-01-20T15:30:00.000Z",
      "updated_at": "2024-01-20T15:30:00.000Z"
    }
  ]
  ```
  - Error (400 Bad Request): Invalid car ID format
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): Car not found or not owned by user
  - Error (500 Internal Server Error): Server-side error

### Update Display Name
- **URL**: `/user/profile/display-name`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
  ```json
  {
    "display_name": "string"
  }
  ```
- **Requirements**:
  - Display name must be between 3 and 50 characters
  - Display name must not match any banned patterns defined in BANNED_USERNAME_PATTERNS environment variable
- **Response**:
  - Success (200 OK): Display name updated successfully
  - Error (400 Bad Request): Invalid display name (too short/long or contains inappropriate content)
  - Error (401 Unauthorized): Invalid or missing token
  - Error (500 Internal Server Error): Failed to update display name

### Delete User Account
- **URL**: `/user/account`
- **Method**: `DELETE`
- **Authentication**: Required
- **Request Body**:
  ```json
  {
    "confirm": true
  }
  ```
- **Requirements**:
  - User must be authenticated
  - The "confirm" field must be explicitly set to true
- **Response**:
  - Success (200 OK):
  ```json
  {
    "message": "Account successfully deleted"
  }
  ```
  - Error (400 Bad Request): Missing or false confirmation
  - Error (401 Unauthorized): Invalid or missing token
  - Error (500 Internal Server Error): Server error during account deletion

When an account is deleted, the following data is permanently removed:
- User profile and personal information
- Friend connections
- Subscription information 
- Authentication tokens
- Likes created by the user

User cars are anonymized rather than deleted completely to maintain the integrity of the trading history and feed items.

**Note**: Account deletion is irreversible. Users will need to create a new account if they wish to use the service again.

## Car Likes
- Cars in collections now include a `likes_count` field that shows the total number of likes received
- Likes on cars are separated from feed item likes and do not generate feed entries
- Use the Likes API endpoints to interact with car likes:
  - `POST /likes/car/{userCarId}` to like a car
  - `DELETE /likes/car/{userCarId}` to unlike a car
  - `GET /likes/car/{userCarId}/check` to check if the current user has liked a car
  - See the [Likes API Documentation](./likes_api.md) for more details

## Notes
- All image paths are relative to the `/images/` endpoint
- Authentication requires a valid JWT token in the Authorization header
- Private collections are only visible to friends
- All timestamps are in ISO 8601 format with UTC timezone
