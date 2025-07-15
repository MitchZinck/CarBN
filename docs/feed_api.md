# Feed API Documentation

## Prerequisites
- Users must be authenticated to access the feed
- Users will only see feed items from themselves and their friends

## Endpoints

### Get Feed
- **URL**: `/feed`
- **Method**: `GET`
- **Authentication**: Required
- **Parameters**:
  - `page_size` (query, optional): Number of items per page (default: 10)
  - `cursor` (query, optional): Pagination cursor from previous response
- **Response**:
  - Success (200 OK):
  ```json
  {
    "items": [
      {
        "id": 123,
        "type": "car_scanned",
        "reference_id": 456,
        "created_at": "2024-01-20T15:04:05Z",
        "user_id": 789,
        "related_user_id": 101,
        "like_count": 5
      }
    ],
    "next_cursor": "base64encodedstring"
  }
  ```
  - Error (401 Unauthorized): Invalid or missing token
  - Error (500 Internal Server Error): Server-side error

### Get Feed Item
- **URL**: `/feed/{feed_item_id}`
- **Method**: `GET`
- **Authentication**: Required
- **Parameters**:
  - `feed_item_id` (path): ID of the feed item to retrieve
- **Response**:
  - Success (200 OK):
  ```json
  {
    "id": 123,
    "type": "car_scanned",
    "reference_id": 456,
    "created_at": "2024-01-20T15:04:05Z",
    "user_id": 789,
    "related_user_id": 101,
    "like_count": 5
  }
  ```
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): Feed item not found
  - Error (500 Internal Server Error): Server-side error

## Feed Item Types
The feed tracks various activities in the system:

- `car_scanned`: When a user successfully scans and adds a car to their collection
  - `reference_id`: ID of the collected car
- `trade_completed`: When a trade between users is completed
  - `reference_id`: ID of the trade
- `friend_accepted`: When a friend request is accepted
  - `reference_id`: ID of the friendship record

## Feed Behavior
- Feed items are ordered by creation date (newest first)
- Users see feed items from:
  - Their own activities
  - Activities of their accepted friends
- Feed uses cursor-based pagination for consistent results
- Each response includes a `next_cursor` if more items are available
- The `next_cursor` can be used in the next request to fetch the next page
- When `next_cursor` is omitted in the response, you've reached the end
- Feed items have unique IDs for tracking and ordering
- Each feed item includes a `like_count` showing total number of likes received
- **Note**: Car likes are intentionally not shown in the feed - when a user likes a car, no feed item is created

## Likes and Feed Items
- Feed items can be liked using the `/likes/{feedItemId}` endpoint (see [Likes API](./likes_api.md))
- Each feed item includes a `like_count` property showing the total number of likes it has received
- To check if the current user has liked a specific feed item, use the `/likes/feed-item/{feedItemId}` endpoint with authentication
- Car likes are tracked separately using `/likes/car/{userCarId}` endpoints and do not appear in the feed
- For full details on liking functionality, refer to the [Likes API Documentation](./likes_api.md)

## Examples

### Request First Page
```http
GET /feed?page_size=5
Authorization: Bearer <access_token>
```

### Request Next Page
```http
GET /feed?page_size=5&cursor=<cursor_from_previous_response>
Authorization: Bearer <access_token>
```

### Response
```json
{
  "items": [
    {
      "id": 789,
      "type": "trade_completed",
      "reference_id": 101,
      "created_at": "2024-01-20T15:04:05Z",
      "user_id": 789,
      "related_user_id": 101,
      "like_count": 5
    },
    {
      "id": 788,
      "type": "car_scanned",
      "reference_id": 202,
      "created_at": "2024-01-20T15:03:05Z",
      "user_id": 789,
      "related_user_id": 101,
      "like_count": 3
    }
  ],
  "next_cursor": "MjAyNC0wMS0yMFQxNTowMzowNVosNzg4"
}
```
