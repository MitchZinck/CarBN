# Authentication API

## Google Sign In

**POST** `/auth/google`

Signs in or registers a user using their Google account. The endpoint will create a new user if the email doesn't exist, otherwise it will update the existing user's last login time.

### Request
```json
{
    "idToken": "string",    // Google ID token obtained from client
    "displayName": "string" // User's chosen display name (3-50 characters)
}
```

### Response
```json
{
    "accessToken": "string",
    "refreshToken": "string",
    "tokenType": "Bearer",
    "expiresIn": 86400  // Token expiry in seconds
}
```

### Status Codes
- 200: Success
- 400: Invalid request (missing token or invalid display name)
- 401: Invalid Google token
- 500: Server error

## Apple Sign In

**POST** `/auth/apple`

Signs in or registers a user using their Apple account. Similar to Google Sign-In, this endpoint will create a new user if the email doesn't exist, otherwise it will update the existing user's last login time.

### Request
```json
{
    "idToken": "string",    // Apple identity token
    "displayName": "string" // User's chosen display name (3-50 characters)
}
```

### Response
```json
{
    "accessToken": "string",
    "refreshToken": "string",
    "tokenType": "Bearer",
    "expiresIn": 86400  // Token expiry in seconds
}
```

### Status Codes
- 200: Success
- 400: Invalid request (missing token or invalid display name)
- 401: Invalid Apple token
- 500: Server error

## Refresh Token

**POST** `/auth/refresh`

Refreshes an expired access token using a valid refresh token.

### Request
```json
{
    "refresh_token": "string"
}
```

### Response
```json
{
    "accessToken": "string",
    "refreshToken": "string",
    "tokenType": "Bearer",
    "expiresIn": 86400  // Token expiry in seconds
}
```

### Status Codes
- 200: Success
- 401: Invalid or expired refresh token
- 500: Server error

## Logout

**POST** `/auth/logout`

Invalidates a refresh token.

### Request
```json
{
    "refresh_token": "string"
}
```

### Response
```json
{
    "message": "Successfully logged out"
}
```

### Status Codes
- 200: Success
- 401: Invalid token
- 500: Server error

## Authorization

All protected endpoints require a valid JWT access token in the Authorization header:

```
Authorization: Bearer <access_token>
```

## Notes for Future Third-Party Auth Providers

The authentication system is designed to be extensible for additional providers:

1. User accounts are linked to auth providers via `auth_provider` and `auth_provider_id` columns
2. The database schema supports multiple auth providers per user through unique constraint
3. New providers (Apple, Facebook, etc.) can be added by:
   - Adding new provider constants in the login service
   - Creating corresponding sign-in handlers and verification logic
   - Implementing provider-specific token validation