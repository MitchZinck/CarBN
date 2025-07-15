# Friends API Documentation

## Endpoints

### Send Friend Request
- **URL**: `/friends/request`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
```json
{
    "friend_id": 123
}
```
- **Response**:
  - Success: `200 OK`
  - Error: `400 Bad Request` or `500 Internal Server Error`

### Respond to Friend Request
- **URL**: `/friends/respond`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
```json
{
    "request_id": 456,
    "response": "accept" | "reject"
}
```
- **Response**:
  - Success: `200 OK`
  - Error: `400 Bad Request` or `500 Internal Server Error`

## Friend Request States
- `pending`: Initial state when a friend request is sent
- `accepted`: State after the recipient accepts the request
- `rejected`: State after the recipient rejects the request

## Related Endpoints

### Get Pending Friend Requests
- **URL**: `/user/friend-requests`
- **Method**: `GET`
- **Authentication**: Required
- **Response**: List of pending friend requests, both ones sent by the current user, and ones received by the current user.

### Get Friends List
- **URL**: `/user/{user_id}/friends`
- **Method**: `GET`
- **Authentication**: Required
- **Query Parameters**:
  - `limit` (optional): Number of friends to return per page (default: 10)
  - `offset` (optional): Number of friends to skip (default: 0)
- **Response**:
  ```json
  {
    "friends": [
      {
        "id": 123,
        "display_name": "John Doe",
        "profile_picture": "path/to/picture.jpg"
      }
    ],
    "total": 50,
    "has_more": true
  }
  ```
  - `friends`: Array of friend objects
  - `total`: Total number of friends
  - `has_more`: Boolean indicating if there are more friends to load
- **Status Codes**:
  - `200 OK`: Successfully retrieved friends
  - `500 Internal Server Error`: Server error

### Check Friendship Status
- **URL**: `/user/{user_id}/is-friend`
- **Method**: `GET`
- **Authentication**: Required
- **Response**:
  ```json
  {
    "is_friend": true
  }
  ```
- **Status Codes**:
  - `200 OK`: Successfully checked friendship status
  - `400 Bad Request`: Invalid user ID
  - `500 Internal Server Error`: Server error

## Feed Integration
When a friend request is accepted, two feed entries are created:
1. For the request sender
2. For the request recipient

Both entries have the type `friend_accepted` and include references to both users involved.
