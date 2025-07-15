# Car Sharing API Documentation

This document outlines the API endpoints used for sharing cars with others through generated links.

## Overview

The car sharing feature allows users to create links that they can share with friends or on social media. These links display a car from the user's collection in a visually appealing format, even to users who don't have the CarBN app installed.

## Base URL

All URLs referenced in the documentation have the following base:

```
https://carbn-test-01.mzinck.com
```

## Authentication

Most endpoints require a valid JWT access token provided in the Authorization header:

```
Authorization: Bearer <access_token>
```

## Endpoints

### Create Share Link

Creates a shareable link for a specific car in the user's collection.

**URL**: `POST /user/cars/{user_car_id}/share`

**Authentication Required**: Yes

**URL Parameters**:
- `user_car_id` - ID of the car to generate a share link for

**Success Response**:
- **Code**: 200 OK
- **Content**:
```json
{
  "share_token": "<token>",
  "share_url": "<share-url>"
}
```

**Error Responses**:

- **Code**: 404 Not Found
  - **Condition**: Car not found or not owned by user
  - **Content**: `{ "error": "car not found or not owned by user" }`

- **Code**: 500 Internal Server Error
  - **Content**: `{ "error": "failed to create share link" }`

**Notes**:
- Share links are valid for 30 days by default
- If a share link already exists for the car and hasn't expired, the existing token will be returned

### Get Shared Car Data

Retrieves the data for a car that has been shared with a token.

**URL**: `GET /share/{share_token}/data`

**Authentication Required**: No

**URL Parameters**:
- `share_token` - The token generated when sharing the car

**Success Response**:
- **Code**: 200 OK
- **Content**:
```json
{
  "make": "Porsche",
  "model": "911 GT3",
  "year": "2021",
  "color": "Guards Red",
  "trim": "RS",
  "horsepower": 518,
  "torque": 347,
  "top_speed": 184,
  "acceleration": 3.2,
  "engine_type": "4.0L Flat-6",
  "drivetrain_type": "RWD",
  "curb_weight": 3197,
  "price": 223250,
  "description": "The most extreme, track-focused version of the 992-generation 911.",
  "rarity": 5,
  "high_res_image": "generated/car_3/premium/1/premium_2_1680123456.jpg",
  "low_res_image": "generated/car_3/premium/1/low_res_2_1680123456.jpg",
  "date_collected": "2025-03-15T12:30:45.123Z",
  "likes_count": 42,
  "view_count": 128,
  "owner_name": "JohnDoe",
  "upgrades": [
    {
      "id": 12,
      "upgrade_type": "premium_image",
      "active": true,
      "metadata": {
        "background_index": 2,
        "timestamp": 1680123456
      },
      "created_at": "2025-03-20T14:15:30.254Z",
      "updated_at": "2025-03-20T14:15:30.254Z"
    }
  ]
}
```

**Error Responses**:

- **Code**: 404 Not Found
  - **Condition**: Share token not found or expired
  - **Content**: `{ "error": "share token not found or expired" }`

- **Code**: 500 Internal Server Error
  - **Content**: `{ "error": "failed to retrieve shared car" }`

**Notes**:
- Each view of this endpoint increments the view counter for analytics
- No authentication is required to access shared car data

### View Shared Car Page

Serves the HTML page for viewing a shared car.

**URL**: `GET /share/{share_token}`

**Authentication Required**: No

**URL Parameters**:
- `share_token` - The token generated when sharing the car

**Success Response**:
- **Code**: 200 OK
- **Content**: HTML page displaying the shared car

**Error Responses**:

- **Code**: 404 Not Found
  - **Condition**: The share token is invalid or expired

**Notes**:
- The page includes meta tags for proper display when sharing on social media
- The frontend JavaScript fetches data from the `/share/{share_token}/data` endpoint
- The page includes social sharing buttons for Twitter, Facebook, and direct link copying

## Database Schema

The shared cars are stored in the `shared_cars` table with the following structure:

```sql
CREATE TABLE shared_cars (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    user_car_id INTEGER NOT NULL REFERENCES user_cars(id),
    token VARCHAR(64) NOT NULL UNIQUE,
    view_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    UNIQUE(user_id, user_car_id)
);
```