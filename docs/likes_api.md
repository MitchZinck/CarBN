# Likes API Documentation

## Endpoints

### Create Like
- **URL**: `/likes/{feedItemId}`
- **Method**: `POST`
- **Authentication**: Required
- **URL Parameters**:
  - `feedItemId`: ID of the feed item to like
- **Response**:
  - Success (200 OK):
  ```json
  {
    "id": 123,
    "user_id": 456,
    "target_id": 789,
    "target_type": "feed_item",
    "created_at": "2024-01-20T15:04:05Z"
  }
  ```
  - Error (400 Bad Request): Invalid feed item ID
  - Error (401 Unauthorized): User not authenticated
  - Error (409 Conflict): User has already liked this feed item
  - Error (500 Internal Server Error): Server error

### Delete Like (Unlike)
- **URL**: `/likes/{feedItemId}`
- **Method**: `DELETE`
- **Authentication**: Required
- **URL Parameters**:
  - `feedItemId`: ID of the feed item to unlike
- **Response**:
  - Success (204 No Content)
  - Error (400 Bad Request): Invalid feed item ID
  - Error (401 Unauthorized): User not authenticated
  - Error (404 Not Found): Like not found
  - Error (500 Internal Server Error): Server error

### Get Feed Item Likes
- **URL**: `/likes/feed-item/{feedItemId}`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `feedItemId`: ID of the feed item
- **Query Parameters**:
  - `cursor` (optional): Pagination cursor from previous response
  - `page_size` (optional, default: 10): Number of likes per page
- **Response**:
  - Success (200 OK):
  ```json
  {
    "items": [
      {
        "id": 123,
        "user_id": 456,
        "target_id": 789,
        "target_type": "feed_item",
        "created_at": "2024-01-20T15:04:05Z"
      }
    ],
    "next_cursor": "base64encodedstring"
  }
  ```
  - Error (400 Bad Request): Invalid feed item ID
  - Error (401 Unauthorized): User not authenticated
  - Error (500 Internal Server Error): Server error
  
### Check If User Liked Feed Item
- **URL**: `/likes/feed-item/{feedItemId}/check`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `feedItemId`: ID of the feed item to check
- **Response**:
  - Success (200 OK):
  ```json
  {
    "liked": true
  }
  ```
  - Error (400 Bad Request): Invalid feed item ID
  - Error (401 Unauthorized): User not authenticated
  - Error (500 Internal Server Error): Server error

### Get User Received Likes
- **URL**: `/likes/user/{userId}`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `userId`: ID of the user whose received likes to fetch
- **Query Parameters**:
  - `cursor` (optional): Pagination cursor from previous response
  - `page_size` (optional, default: 10): Number of likes per page
- **Response**:
  - Success (200 OK):
  ```json
  {
    "items": [
      {
        "id": 123,
        "user_id": 456,
        "target_id": 789,
        "target_type": "feed_item",
        "created_at": "2024-01-20T15:04:05Z"
      }
    ],
    "next_cursor": "base64encodedstring"
  }
  ```
  - Error (400 Bad Request): Invalid user ID
  - Error (401 Unauthorized): User not authenticated
  - Error (500 Internal Server Error): Server error

### Create Car Like
- **URL**: `/likes/car/{userCarId}`
- **Method**: `POST`
- **Authentication**: Required
- **URL Parameters**:
  - `userCarId`: ID of the user car to like
- **Response**:
  - Success (200 OK):
  ```json
  {
    "id": 123,
    "user_id": 456,
    "target_id": 789,
    "target_type": "user_car",
    "created_at": "2024-01-20T15:04:05Z"
  }
  ```
  - Error (400 Bad Request): Invalid user car ID
  - Error (401 Unauthorized): User not authenticated
  - Error (409 Conflict): User has already liked this car
  - Error (500 Internal Server Error): Server error

### Delete Car Like (Unlike)
- **URL**: `/likes/car/{userCarId}`
- **Method**: `DELETE`
- **Authentication**: Required
- **URL Parameters**:
  - `userCarId`: ID of the user car to unlike
- **Response**:
  - Success (204 No Content)
  - Error (400 Bad Request): Invalid user car ID
  - Error (401 Unauthorized): User not authenticated
  - Error (404 Not Found): Like not found
  - Error (500 Internal Server Error): Server error

### Get Car Likes
- **URL**: `/likes/car/{userCarId}`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `userCarId`: ID of the user car
- **Query Parameters**:
  - `cursor` (optional): Pagination cursor from previous response
  - `page_size` (optional, default: 10): Number of likes per page
- **Response**:
  - Success (200 OK):
  ```json
  {
    "items": [
      {
        "id": 123,
        "user_id": 456,
        "target_id": 789,
        "target_type": "user_car",
        "created_at": "2024-01-20T15:04:05Z"
      }
    ],
    "next_cursor": "base64encodedstring"
  }
  ```
  - Error (400 Bad Request): Invalid user car ID
  - Error (401 Unauthorized): User not authenticated
  - Error (500 Internal Server Error): Server error

### Check If User Liked Car
- **URL**: `/likes/car/{userCarId}/check`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `userCarId`: ID of the user car to check
- **Response**:
  - Success (200 OK):
  ```json
  {
    "liked": true
  }
  ```
  - Error (400 Bad Request): Invalid user car ID
  - Error (401 Unauthorized): User not authenticated
  - Error (500 Internal Server Error): Server error

### Get Car Likes Count
- **URL**: `/likes/car/{userCarId}/count`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `userCarId`: ID of the user car
- **Response**:
  - Success (200 OK):
  ```json
  {
    "count": 42
  }
  ```
  - Error (400 Bad Request): Invalid user car ID
  - Error (401 Unauthorized): User not authenticated
  - Error (500 Internal Server Error): Server error

## Pagination
- The API uses cursor-based pagination for consistent results
- Each response includes a `next_cursor` if more items are available
- Pass the `next_cursor` in the next request to get the next page
- When `next_cursor` is omitted in the response, you've reached the end
- Likes are ordered by creation date (newest first)

## Example Usage

### Like a Feed Item
```http
POST /likes/123
Authorization: Bearer <access_token>
```

### Unlike a Feed Item
```http
DELETE /likes/123
Authorization: Bearer <access_token>
```

### Get Feed Item Likes (First Page)
```http
GET /likes/feed-item/123?page_size=5
Authorization: Bearer <access_token>
```

### Get Feed Item Likes (Next Page)
```http
GET /likes/feed-item/123?page_size=5&cursor=<cursor_from_previous_response>
Authorization: Bearer <access_token>
```

### Get User's Received Likes
```http
GET /likes/user/456?page_size=10
Authorization: Bearer <access_token>
```

### Like a Car
```http
POST /likes/car/123
Authorization: Bearer <access_token>
```

### Unlike a Car
```http
DELETE /likes/car/123
Authorization: Bearer <access_token>
```

### Check if Current User Liked a Car
```http
GET /likes/car/123/check
Authorization: Bearer <access_token>
```

### Get Car Likes Count
```http
GET /likes/car/123/count
Authorization: Bearer <access_token>
```

### Get Users Who Liked a Car
```http
GET /likes/car/123?page_size=20
Authorization: Bearer <access_token>
```

## Feed Item Integration
- When retrieving feed items through the Feed API, each item includes a `like_count` field showing the total number of likes it has received
- You can use this count to display like statistics without making additional API calls
- To check if the current user has liked a specific feed item, you'll need to query the likes endpoint

## User Car Integration
- When retrieving user cars through the User API, each car includes a `likes_count` field showing the total number of likes it has received
- The `likes_count` is automatically updated when users like or unlike a car
- To check if the current user has liked a specific car, use the `/likes/car/{userCarId}/check` endpoint
- Unlike feed item likes, car likes are NOT shown in the activity feed