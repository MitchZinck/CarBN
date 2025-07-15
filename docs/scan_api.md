# Scan API Documentation

## Prerequisites
- Users must have scan credits available to perform a scan
- Each scan consumes one scan credit
- New users start with 6 scan credits
- Additional scan credits are available through subscription or by purchasing scan packs

## Endpoints

### Scan Car Image
- **URL**: `/scan`
- **Method**: `POST`
- **Authentication**: Required
- **Request Body**:
  - `image`: Base64 encoded image of the car

- **Response**:
  - `scan_id`: Unique identifier for the scan
  - `status`: Status of the scan (e.g., `pending`, `completed`, `failed`)
  - `result`: Scan result data (if available)
  - `error`: Error message (if any)

- **Error Codes**:
  - `400`: Bad Request - Invalid input data
  - `401`: Unauthorized - Authentication failed
  - `403`: Forbidden - Insufficient scan credits
  - `500`: Internal Server Error - An unexpected error occurred

## Obtaining Scan Credits
There are three ways to obtain scan credits:
1. **New User Credits**: New users start with 6 scan credits automatically
2. **Subscription**: Subscribe to get monthly credits (see [Subscription API](./subscription_api.md))
3. **Scan Packs**: Purchase one-time scan credit packs (see below)

## Scan Pack Purchasing
For details on purchasing scan packs, see the [Scan Pack Purchase API](./scan_pack_api.md).
