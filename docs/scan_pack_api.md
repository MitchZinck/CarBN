# Scan Pack Purchase API

## Overview
Scan packs allow users to purchase additional scan credits as a one-time purchase, regardless of their subscription status. These packs are available in different sizes with varying amounts of credits.

## Available Scan Packs
| Name | Product ID | Credits | Description |
|------|------------|---------|-------------|
| Starter Pack | com.mzinck.carbn.iap.scanpack.tencredits | 10 | Beginner pack with 10 scan credits |
| Value Pack | com.mzinck.carbn.iap.scanpack.fiftycredits | 50 | Medium-sized pack with 50 scan credits |
| Mega Pack | com.mzinck.carbn.iap.scanpack.onehundredcredits | 100 | Large pack with 100 scan credits |

## Endpoints

### Get Available Scan Pack Products
Lists all available scan pack products that can be purchased.

- **URL**: `/subscription/products?type=scanpack`
- **Method**: `GET`
- **Authentication**: Required

#### Query Parameters
- `platform`: Filter products by platform ("apple" or "google"). Optional.
- `type`: Filter by product type. Set to "scanpack" to get only scan packs.

#### Response
```json
[
  {
    "id": 4,
    "product_id": "com.mzinck.carbn.iap.scanpack.tencredits",
    "name": "Starter Pack",
    "tier": "none",
    "platform": "apple",
    "duration_days": 0,
    "scan_credits": 10,
    "type": "scanpack"
  },
  {
    "id": 5,
    "product_id": "com.mzinck.carbn.iap.scanpack.fiftycredits",
    "name": "Value Pack",
    "tier": "none",
    "platform": "apple",
    "duration_days": 0,
    "scan_credits": 50,
    "type": "scanpack"
  },
  {
    "id": 6,
    "product_id": "com.mzinck.carbn.iap.scanpack.onehundredcredits",
    "name": "Mega Pack",
    "tier": "none",
    "platform": "apple",
    "duration_days": 0,
    "scan_credits": 100,
    "type": "scanpack"
  }
]
```

### Purchase Scan Pack

- **URL**: `/scanpack/purchase`
- **Method**: `POST`
- **Authentication**: Required

#### Request Body
```json
{
  "receipt_data": "string or transaction ID",
  "platform": "apple"
}
```

- `receipt_data`: For Apple, this can be either:
  - A transaction ID from StoreKit 2
  - A receipt data string from the legacy StoreKit 1 API
- `platform`: "apple" or "google" (Google Play integration coming soon)

#### Response
```json
{
  "success": true,
  "message": "Successfully purchased Starter Pack with 10 scan credits",
  "subscription": {
    "is_active": false,
    "tier": "none",
    "scan_credits_remaining": 16
  }
}
```

## Client Integration Guide

### Apple In-App Purchase Integration with StoreKit 2

1. **Set up products in App Store Connect**:
   - Create three products with these identifiers:
     - `com.mzinck.carbn.iap.scanpack.tencredits`
     - `com.mzinck.carbn.iap.scanpack.fiftycredits`
     - `com.mzinck.carbn.iap.scanpack.onehundredcredits`
   - Set the product type as "Consumable" in App Store Connect

2. **Fetch Available Products**:
   ```swift
   // Swift example using StoreKit 2
   import StoreKit
   
   func fetchScanPackProducts() async throws -> [Product] {
     let productIDs = [
       "com.mzinck.carbn.iap.scanpack.tencredits",
       "com.mzinck.carbn.iap.scanpack.fiftycredits",
       "com.mzinck.carbn.iap.scanpack.onehundredcredits"
     ]
     return try await Product.products(for: productIDs)
   }
   ```

3. **Process a Purchase**:
   ```swift
   // Swift example using StoreKit 2
   import StoreKit
   
   func purchaseScanPack(product: Product) async throws {
     // Purchase the product using StoreKit 2
     let result = try await product.purchase()
     
     switch result {
     case .success(let verification):
       // Get transaction after verification
       let transaction = try checkVerified(verification)
       
       // Send transaction ID to backend
       try await sendScanPackTransactionToServer(transactionId: transaction.id)
       
       // Finish the transaction
       await transaction.finish()
       
     case .userCancelled:
       throw PurchaseError.userCancelled
     default:
       throw PurchaseError.failed
     }
   }
   
   func sendScanPackTransactionToServer(transactionId: UInt64) async throws {
     // Format transaction ID as string
     let transactionIdString = "\(transactionId)"
     
     // Send to your server
     let url = URL(string: "https://yourserver.com/scanpack/purchase")!
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

## Important Notes

1. Unlike subscriptions, scan pack purchases:
   - Are one-time purchases that can be bought multiple times
   - Do not expire
   - Add credits to the user's existing balance
   - Can be purchased regardless of subscription status
   
2. Transaction IDs are tracked to prevent duplicate credit grants from the same purchase

3. When using both subscriptions and scan packs:
   - Subscription renewal adds credits based on the tier
   - Scan pack credits are added to the user's existing balance
   - Credits from both sources are stored and consumed from the same pool

4. There are no refunds for used scan credits. If a purchase is refunded through Apple, the server will not automatically deduct already-used credits.