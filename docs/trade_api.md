# Trade API Documentation

## Prerequisites
- Both users involved in a trade must have an active subscription
- Trading is not allowed for users without an active subscription

## Endpoints

### Create Trade Request
- **URL**: `/trade/request`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
```json
{
    "user_id_to": 123,
    "user_from_user_car_ids": [1, 2, 3],
    "user_to_user_car_ids": [4, 5]
}
```
- **Response**:
  - Success: `200 OK`
  - Error: `400 Bad Request` - Invalid request data
  - Error: `500 Internal Server Error` - Server error or car ownership verification failed

### Respond to Trade Request
- **URL**: `/trade/respond`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
```json
{
    "trade_id": 456,
    "response": "accept" | "decline"
}
```
- **Response**:
  - Success: `200 OK`
  - Error: `400 Bad Request` - Invalid response type
  - Error: `500 Internal Server Error` - Failed to process trade response

### Get Trade History
- **URL**: `/trade/history`
- **Method**: `GET`
- **Authentication**: Required
- **Query Parameters**:
  - `page` (optional, default: 1): The page number to fetch
  - `page_size` (optional, default: 10): Number of trades per page
- **Response**:
```json
{
    "trades": [
        {
            "id": 123,
            "user_id_from": 456,
            "user_id_to": 789,
            "status": "pending"|"accepted"|"declined",
            "user_from_user_car_ids": [1, 2, 3],
            "user_to_user_car_ids": [4, 5],
            "created_at": "2024-01-20T15:30:00Z"
        }
    ],
    "total_count": 50
}
```
- **Response Codes**:
  - Success: `200 OK`
  - Error: `500 Internal Server Error` - Failed to fetch trade history

### Get Specific Trade
- **URL**: `/trade/{trade_id}`
- **Method**: `GET`
- **Authentication**: Required
- **URL Parameters**:
  - `trade_id`: The ID of the trade to fetch
- **Response**:
```json
{
    "id": 123,
    "user_id_from": 456,
    "user_id_to": 789,
    "status": "pending"|"accepted"|"declined",
    "user_from_user_car_ids": [1, 2, 3],
    "user_to_user_car_ids": [4, 5],
    "created_at": "2024-01-20T15:30:00Z",
    "traded_at": "2024-01-20T16:30:00Z"
}
```
- **Response Codes**:
  - Success: `200 OK`
  - Error: `400 Bad Request` - Invalid trade ID format
  - Error: `404 Not Found` - Trade not found
  - Error: `500 Internal Server Error` - Server error

## Trade States
- `pending`: Initial state when a trade request is created
- `accepted`: State after the recipient accepts the trade
- `declined`: State after the recipient rejects the trade

## Car Ownership
- When a trade is accepted, car ownership is automatically transferred between users
- Both users must own the cars they are offering in the trade
- Cars can only be involved in one pending trade at a time
