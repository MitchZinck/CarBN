# Car Upgrades Documentation

## Upgrade System

The car upgrade system is designed to be extensible, supporting various types of upgrades that can be applied to cars in a user's collection. Each upgrade is tracked in the `car_upgrades` table with the following structure:

```sql
car_upgrades (
    id              SERIAL PRIMARY KEY,
    user_car_id     INT NOT NULL,
    upgrade_type    VARCHAR(50) NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB,
    created_at      TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
)
```

### Current Upgrade Types

#### Premium Image (`premium_image`)
- Cost: 5000 currency
- Features:
  - Dynamic action shots in exclusive environments
  - Original images can be restored at any time for free
  - Image upgrades are per-user - the original car in the database remains unchanged
- Metadata structure:
  ```json
  {
    "original_low_res": "path/to/original/low_res.jpg",
    "original_high_res": "path/to/original/high_res.jpg"
  }
  ```

### Future Upgrade Types (Planned)
- Performance upgrades
- Visual customization
- Special effects
- Custom backgrounds

## Endpoints

### Upgrade Car Image
- **URL**: `/user/cars/{user_car_id}/upgrade-image`
- **Method**: `POST`
- **Authentication**: Required
- **URL Parameters**:
  - `user_car_id`: ID of the car to upgrade
- **Response**:
  - Success (200 OK):
  ```json
  {
    "message": "car image upgraded successfully",
    "remaining_currency": 2500,
    "high_res_image": "car_1/blue/high_res.jpg",
    "low_res_image": "car_1/blue/low_res.jpg"
  }
  ```
  - Error (400 Bad Request): Invalid car ID format
  - Error (401 Unauthorized): Invalid or missing token
  - Error (402 Payment Required): Insufficient currency (requires 5000) or not subscribed
  - Error (404 Not Found): Car not found or not owned by user
  - Error (500 Internal Server Error): Server error

### Revert Car Image
- **URL**: `/user/cars/{user_car_id}/revert-image`
- **Method**: `POST`
- **Authentication**: Required
- **URL Parameters**:
  - `user_car_id`: ID of the car to revert
- **Response**:
  - Success (200 OK):
  ```json
  {
    "message": "car image reverted successfully",
    "high_res_image": "car_1/blue/high_res.jpg",
    "low_res_image": "car_1/blue/low_res.jpg"
  }
  ```
  - Error (400 Bad Request): Invalid car ID format
  - Error (401 Unauthorized): Invalid or missing token
  - Error (404 Not Found): Car not found or not owned by user
  - Error (500 Internal Server Error): Server error

### Get Car Upgrades
See the User API documentation for details about the upgrades endpoint.