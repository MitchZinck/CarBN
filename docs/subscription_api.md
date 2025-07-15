# Subscription System

The subscription system manages user subscriptions and scan credits in CarBN.

## Features

- Three subscription tiers: Basic, Standard, and Premium
- Track active subscription status
- Manage scan credits for users
- Restrict trading features to subscribed users
- Apple App Store Server API integration (Google Pay coming soon)
- Real-time subscription status updates via App Store Server Notifications V2

## Subscription Tiers

| Tier     | Benefits                           | Scan Credits |
|----------|----------------------------------- |--------------|
| Basic    | Trading + Car Image Upgrades       | 30           |
| Standard | Trading + Car Image Upgrades       | 60           |
| Premium  | Trading + Car Image Upgrades       | 100          |

## Endpoints

### GET /user/subscription

Returns the current user's subscription status and scan credits.

**Authentication Required**: Yes (JWT Token)

#### Response

```json
{
  "is_active": boolean,
  "tier": string,
  "subscription_start": string (ISO date) | null,
  "subscription_end": string (ISO date) | null,
  "scan_credits_remaining": number
}
```

#### Example Response

```json
{
  "is_active": true,
  "tier": "premium",
  "subscription_start": "2024-01-01T00:00:00Z",
  "subscription_end": "2024-12-31T23:59:59Z",
  "scan_credits_remaining": 100
}
```

### GET /user/{user_id}/subscription/status

Returns whether a specific user has an active subscription.

**Authentication Required**: Yes (JWT Token)

#### Parameters

- `user_id`: ID of the user to check (path parameter)

#### Response

```json
{
  "is_active": boolean
}
```

### GET /subscription/products

Returns a list of available subscription products.

**Authentication Required**: Yes (JWT Token)

#### Query Parameters

- `platform`: Filter products by platform ("apple" or "google"). If omitted, returns all products.

#### Response

```json
[
  {
    "id": number,
    "product_id": string,
    "name": string,
    "tier": string,
    "platform": string,
    "duration_days": number,
    "scan_credits": number
  }
]
```

#### Example Response

```json
[
  {
    "id": 1,
    "product_id": "com.mzinck.carbn.subscription.monthly.basic1",
    "name": "Basic Subscription",
    "tier": "basic",
    "platform": "apple",
    "duration_days": 30,
    "scan_credits": 30
  },
  {
    "id": 2,
    "product_id": "com.mzinck.carbn.subscription.monthly.standard",
    "name": "Standard Subscription",
    "tier": "standard",
    "platform": "apple",
    "duration_days": 30,
    "scan_credits": 60
  },
  {
    "id": 3,
    "product_id": "com.mzinck.carbn.subscription.monthly.premium",
    "name": "Premium Subscription",
    "tier": "premium",
    "platform": "apple",
    "duration_days": 30,
    "scan_credits": 100
  }
]
```

### POST /subscription/purchase

Process a subscription purchase.

**Authentication Required**: Yes (JWT Token)

#### Request Body

```json
{
  "receipt_data": string,
  "platform": string
}
```

- `receipt_data`: For Apple, this can be either:
  - A transaction ID from StoreKit 2
  - A receipt data string from the legacy StoreKit 1 API (for backward compatibility)
- `platform`: "apple" or "google" (Google Play integration coming soon)

#### Response

```json
{
  "success": boolean,
  "message": string,
  "subscription": {
    "is_active": boolean,
    "tier": string,
    "subscription_start": string,
    "subscription_end": string,
    "scan_credits_remaining": number
  }
}
```

#### Example Response

```json
{
  "success": true,
  "message": "Successfully processed premium subscription",
  "subscription": {
    "is_active": true,
    "tier": "premium",
    "subscription_start": "2024-06-15T10:30:00Z",
    "subscription_end": "2024-07-15T10:30:00Z",
    "scan_credits_remaining": 100
  }
}
```

### POST /webhook/apple/subscription

Webhook endpoint for App Store Server Notifications V2.

**Authentication Required**: No (This endpoint is called by Apple's servers)

#### Request Body

The request body contains an App Store Server Notification V2 payload as described in [Apple's documentation](https://developer.apple.com/documentation/appstoreservernotifications/responsebody).

#### Response

Returns HTTP 200 OK to acknowledge receipt of the notification.

## Client Integration Guide

### Apple In-App Purchase Integration with StoreKit 2

1. **Set up products in App Store Connect**:
   - Create three products with these identifiers:
     - `com.mzinck.carbn.subscription.monthly.basic1`
     - `com.mzinck.carbn.subscription.monthly.standard`
     - `com.mzinck.carbn.subscription.monthly.premium`
   - Set up App Store Server Notifications V2 in App Store Connect
   - Configure the server endpoint URL: `https://yourserver.com/webhook/apple/subscription`

2. **Create App Store Server API Keys**:
   - Generate a private key in App Store Connect (Users and Access > Keys)
   - Store the private key securely on your server
   - Set the following environment variables:
     - `APPLE_ISSUER_ID`: Your issuer ID from App Store Connect
     - `APPLE_KEY_ID`: Your key ID from App Store Connect
     - `APPLE_PRIVATE_KEY_PATH`: Path to the private key file

3. **Fetch Available Products**:

   ```swift
   // Swift example using StoreKit 2
   import StoreKit
   
   func fetchProducts() async throws -> [Product] {
     let productIDs = [
       "com.mzinck.carbn.subscription.monthly.basic1",
       "com.mzinck.carbn.subscription.monthly.standard",
       "com.mzinck.carbn.subscription.monthly.premium"
     ]
     return try await Product.products(for: productIDs)
   }
   ```

4. **Process a Purchase**:

   ```swift
   // Swift example using StoreKit 2
   import StoreKit
   
   func purchase(product: Product) async throws {
     // Purchase the product using StoreKit 2
     let result = try await product.purchase()
     
     switch result {
     case .success(let verification):
       // Get transaction after verification
       let transaction = try checkVerified(verification)
       
       // Send transaction ID to backend
       try await sendTransactionToServer(transactionId: transaction.id)
       
       // Finish the transaction
       await transaction.finish()
       
     case .userCancelled:
       throw PurchaseError.userCancelled
     default:
       throw PurchaseError.failed
     }
   }
   
   func sendTransactionToServer(transactionId: UInt64) async throws {
     // Format transaction ID as string
     let transactionIdString = "\(transactionId)"
     
     // Send to your server
     let url = URL(string: "https://yourserver.com/subscription/purchase")!
     var request = URLRequest(url: url)
     request.httpMethod = "POST"
     request.addValue("Bearer \(yourAuthToken)", forHTTPHeaderField: "Authorization")
     request.addValue("application/json", forHTTPHeaderField: "Content-Type")
     
     let body: [String: Any] = [
       "receipt_data": transactionIdString,
       "platform": "apple"
     ]
     request.httpBody = try JSONSerialization.data(withJSONObject: body)
     
     let (data, response) = try await URLSession.shared.data(for: request)
     // Handle response...
   }
   ```

### Handling Subscription Status

Check if a user has an active subscription before allowing trades or image upgrades:

```javascript
// Example using fetch
const checkSubscriptionStatus = async (userId) => {
  const response = await fetch(`/user/${userId}/subscription/status`, {
    headers: {
      'Authorization': `Bearer ${yourAuthToken}`
    }
  });
  const data = await response.json();
  return data.is_active;
};
```

## Business Rules

1. New users start with 6 scan credits
2. Trading requires an active subscription for both parties
3. Users cannot trade with users who don't have an active subscription
4. Scan credits are consumed when successfully scanning a car
5. Subscription tiers and their benefits:
   - Basic: 30 scan credits, access to trading and car image upgrades
   - Standard: 60 scan credits, access to trading and car image upgrades
   - Premium: 100 scan credits, access to trading and car image upgrades
6. When purchasing a subscription, scan credits are added to existing credits
7. All subscriptions last for 30 days

## App Store Server API Error Codes

The following error codes may be returned when using the App Store Server API:

- `4040001` to `4040010`: Various "not found" errors (transaction not found, etc.)
- `5000001` to `5000007`: Internal server errors
- `4000001` to `4000007`: Various client errors (invalid request, bad parameters, etc.)

For more details, refer to [Apple's App Store Server API documentation](https://developer.apple.com/documentation/appstoreserverapi/error_codes).

## App Store Server Notifications V2

CarBN now supports App Store Server Notifications V2, which provides real-time updates about subscription status changes directly from Apple. This includes:

- New subscriptions (`SUBSCRIBED`)
- Renewals (`DID_RENEW`)
- Expirations (`EXPIRED`)
- Failed renewals (`DID_FAIL_TO_RENEW`)
- Refunds (`REFUND`)
- Price increases (`PRICE_INCREASE`)
- And other notification types

These notifications are automatically processed by our server and update the user's subscription status accordingly, without requiring any client-side action.

For more information, see [Apple's documentation on App Store Server Notifications](https://developer.apple.com/documentation/appstoreservernotifications).
