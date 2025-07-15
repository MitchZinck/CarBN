# CarBN Backend

A Go-based backend service for CarBN, an iOS car-collecting app that uses AI to recognize and catalog cars from photos. Users can build collections, trade with friends, and participate in a social feed system.

## Features

- **Car Recognition**: AI-powered car identification from photos
- **User Collections**: Personal car card collections with rarity systems
- **Social Features**: Friend system with requests and feeds
- **Trading System**: Car trading between friends (subscription required)
- **Subscription Management**: Apple App Store integration for subscriptions and scan packs
- **Feed System**: Activity feeds showing friend interactions
- **Likes System**: Like cars and feed items
- **Car Upgrades**: Premium image upgrades for collected cars
- **Secure Authentication**: Google and Apple Sign-In support

## Tech Stack

- **Backend**: Go 1.23+
- **Database**: PostgreSQL
- **Authentication**: JWT tokens with Google/Apple OAuth
- **AI Integration**: Google Generative AI for car recognition
- **Payment Processing**: Apple App Store Server-to-Server notifications

## Quick Start

### Prerequisites

- Go 1.23 or later
- PostgreSQL 12+
- Environment variables configured (see `.env.example`)

### Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd CarBN_backend
```

2. Install dependencies:
```bash
go mod download
```

3. Set up your environment variables:
```bash
cp .env.example .env
# Edit .env with your configuration
```

4. Initialize the database:
```bash
# Create your PostgreSQL database
# Run the SQL schema
psql -d your_database -f postgres/sql/create.sql
```

5. Start the development server:
```bash
go run main.go
```

The server will start on `http://localhost:8080`


### Base URL
```
Development: http://localhost:8080
```

### Authentication
Most endpoints require a Bearer token in the Authorization header:
```
Authorization: Bearer <access_token>
```