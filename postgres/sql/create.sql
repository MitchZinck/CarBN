-- Create the "User" table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255),
    auth_provider VARCHAR(20) DEFAULT 'google',
    auth_provider_id VARCHAR(255),
    full_name VARCHAR(255),
    display_name VARCHAR(255),
    followers_count INT DEFAULT 0,
    profile_picture VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    is_private BOOLEAN NOT NULL DEFAULT FALSE,
    currency INT NOT NULL DEFAULT 0,
    last_login TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    is_private_email BOOLEAN DEFAULT false,
    CONSTRAINT users_email_provider_key UNIQUE (email, auth_provider),
    CONSTRAINT unique_provider_id UNIQUE (auth_provider, auth_provider_id)
);

-- Create index for user lookup by email which is used in auth context
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Create the "Car" table
CREATE TABLE IF NOT EXISTS cars (
    id SERIAL PRIMARY KEY,                -- Unique identifier
    make VARCHAR(100) NOT NULL,              -- Manufacturer (e.g., Tesla, BMW)
    model VARCHAR(100) NOT NULL,             -- Model name (e.g., Model S, X5)
    trim VARCHAR(100),                       -- Specific trim or variant (e.g., Sport, Long Range)
    year VARCHAR(20),                       -- Manufacturing year (as string)

    -- Performance Specs
    horsepower INT,                          -- Horsepower (e.g., 450)
    torque INT,                              -- Torque (in Nm or lb-ft)
    top_speed INT,                           -- Top speed (in km/h or mph)
    acceleration DECIMAL,                      -- 0-60 mph or 0-100 km/h time (in seconds)

    -- Engine Specs
    engine_type VARCHAR(50),               -- Engine type (e.g., V6, V8, Inline-4)
    fuel_type VARCHAR(50),                  -- Fuel type (e.g., Gasoline, Diesel, Electric)
    displacement FLOAT,                     -- Engine displacement (in liters, e.g., 3.0)
    cylinder_count INT,                     -- Number of cylinders (e.g., 4, 6, 8)
    forced_induction BOOLEAN,               -- Whether the engine has turbocharging/supercharging
    hybrid BOOLEAN,                         -- Whether the car is a hybrid
    drivetrain_type VARCHAR(50),           -- Drivetrain type (e.g., FWD, RWD, AWD)

    -- Electric Vehicle (EV) Specs
    battery_capacity FLOAT,                 -- Battery capacity (in kWh)
    range FLOAT,                            -- Range on a full charge (in miles or km)
    charging_time FLOAT,                    -- Charging time (in hours)

    -- Transmission
    transmission_type VARCHAR(50),         -- Transmission type (e.g., Automatic, Manual, CVT)
    gear_count INT,                        -- Number of gears (e.g., 6, 8)

    -- Dimensions and Weight
    curb_weight DECIMAL,                     -- Weight (in kg or lbs)
    length FLOAT,                          -- Length (in mm or inches)
    width FLOAT,                           -- Width (in mm or inches)
    height FLOAT,                          -- Height (in mm or inches)
    wheelbase FLOAT,                       -- Wheelbase (in mm or inches)
    ground_clearance FLOAT,                -- Ground clearance (in mm or inches)

    -- Exterior
    body_type VARCHAR(50),                 -- Body type (e.g., Sedan, SUV, Coupe, Truck)
    doors INT,                             -- Number of doors (e.g., 2, 4, 5)
    wheel_size FLOAT,                      -- Wheel size (in inches, e.g., 18, 20)

    -- Miscellaneous
    price DECIMAL,                           -- Price (in USD or other currency)
    color_images JSONB,                    -- Store color-based images as {"color": {"low_res": "url", "high_res": "url"}}
    description TEXT,                      -- Additional description or highlights
    rarity INTEGER DEFAULT 0,                  -- Rarity level (0-5)
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Create indices for common car queries
CREATE INDEX IF NOT EXISTS idx_cars_make_model ON cars(make, model);
CREATE INDEX IF NOT EXISTS idx_cars_year ON cars(year);
CREATE INDEX IF NOT EXISTS idx_cars_created_at ON cars(created_at);

-- Create the "UserCars" (junction) table
CREATE TABLE IF NOT EXISTS user_cars (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL CHECK (user_id >= 0), -- ID 0 is reserved for sold cars
    car_id INT NOT NULL,
    color VARCHAR(50) NOT NULL,
    date_collected TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    low_res_image TEXT,
    high_res_image TEXT,
    FOREIGN KEY (car_id) REFERENCES cars (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Create indices for user_cars lookups
CREATE INDEX IF NOT EXISTS idx_user_cars_user_id ON user_cars(user_id);
CREATE INDEX IF NOT EXISTS idx_user_cars_car_id ON user_cars(car_id);
CREATE INDEX IF NOT EXISTS idx_user_cars_created_at ON user_cars(created_at);

-- Create the "Friendships" table
CREATE TABLE IF NOT EXISTS friends (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    friend_id INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (friend_id) REFERENCES users (id) ON DELETE CASCADE,
    UNIQUE (user_id, friend_id),
    CONSTRAINT no_self_friendship CHECK (user_id <> friend_id)
);

-- Create indices for friend lookups
CREATE INDEX IF NOT EXISTS idx_friends_user_id ON friends(user_id, status);
CREATE INDEX IF NOT EXISTS idx_friends_friend_id ON friends(friend_id, status);
CREATE INDEX IF NOT EXISTS idx_friends_status ON friends(status);

-- Create the "Trades" table
CREATE TABLE IF NOT EXISTS trades (
    id SERIAL PRIMARY KEY,
    user_id_from INT NOT NULL,
    user_id_to INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    user_from_user_car_ids INT[] NOT NULL,  -- Added column
    user_to_user_car_ids INT[] NOT NULL,    -- Added column
    traded_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id_from) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id_to) REFERENCES users (id) ON DELETE CASCADE
);

-- Create indices for trade lookups
CREATE INDEX IF NOT EXISTS idx_trades_user_from ON trades(user_id_from, status);
CREATE INDEX IF NOT EXISTS idx_trades_user_to ON trades(user_id_to, status);
CREATE INDEX IF NOT EXISTS idx_trades_status ON trades(status);

-- Create the "Feed" table
CREATE TABLE IF NOT EXISTS feed (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'scan', 'trade', 'friend_accepted'
    reference_id INT NOT NULL, -- ID of the related scan or trade
    related_user_id INT, -- ID of the related user (for friend requests, trades)
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (related_user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Create indices for feed queries
CREATE INDEX IF NOT EXISTS idx_feed_user_id ON feed(user_id);
CREATE INDEX IF NOT EXISTS idx_feed_created_at ON feed(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feed_type ON feed(type);

-- Create the "Likes" table
CREATE TABLE IF NOT EXISTS likes (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    feed_item_id INT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (feed_item_id) REFERENCES feed (id) ON DELETE CASCADE,
    UNIQUE (user_id, feed_item_id)  -- Prevent duplicate likes
);

-- Create indices for like lookups
CREATE INDEX IF NOT EXISTS idx_likes_feed_item_id ON likes(feed_item_id);
CREATE INDEX IF NOT EXISTS idx_likes_user_id ON likes(user_id);
CREATE INDEX IF NOT EXISTS idx_likes_created_at ON likes(created_at DESC);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token VARCHAR(255) PRIMARY KEY,
    user_id INT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Create indices for refresh token lookups
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

CREATE OR REPLACE FUNCTION cleanup_expired_tokens() RETURNS void AS $$
BEGIN
    DELETE FROM refresh_tokens WHERE expires_at < CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE user_subscriptions (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL UNIQUE,
    subscription_start TIMESTAMPTZ,
    subscription_end TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT FALSE,
    scan_credits_remaining INT DEFAULT 6,
    tier VARCHAR(20) DEFAULT 'none',  -- 'none', 'basic', 'standard', 'premium'
    receipt_data TEXT,                -- Store Apple receipt data
    original_transaction_id TEXT,     -- Apple original transaction identifier
    latest_receipt TEXT,              -- Latest Apple receipt
    platform VARCHAR(20),             -- 'apple', 'google', etc.
    transaction_id VARCHAR(255),      -- Transaction ID for subscription
    environment VARCHAR(50),          -- 'Production' or 'Sandbox'
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create indices for user subscriptions
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_id ON user_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_transaction_id ON user_subscriptions(transaction_id);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_original_transaction_id ON user_subscriptions(original_transaction_id);

-- Create table for subscription products
CREATE TABLE subscription_products (
    id SERIAL PRIMARY KEY,
    product_id VARCHAR(100) NOT NULL UNIQUE, -- Apple/Google product ID
    name VARCHAR(50) NOT NULL,               -- Display name
    tier VARCHAR(20) NOT NULL,               -- 'basic', 'standard', 'premium'
    platform VARCHAR(20) NOT NULL,           -- 'apple', 'google'
    duration_days INT NOT NULL,              -- Subscription duration
    scan_credits INT NOT NULL,               -- Number of scan credits
    type VARCHAR(20) NOT NULL DEFAULT 'subscription', -- Type: 'subscription' or 'scanpack'
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Insert default Apple subscription products
INSERT INTO subscription_products 
    (product_id, name, tier, platform, duration_days, scan_credits) 
VALUES
    ('com.mzinck.carbn.subscription.monthly.basic1', 'Basic Subscription', 'basic', 'apple', 30, 30),
    ('com.mzinck.carbn.subscription.monthly.standard', 'Standard Subscription', 'standard', 'apple', 30, 60),
    ('com.mzinck.carbn.subscription.monthly.premium', 'Premium Subscription', 'premium', 'apple', 30, 100);

-- Insert scan pack products for Apple
INSERT INTO subscription_products (
    product_id, name, tier, platform, duration_days, scan_credits, type
) VALUES 
    ('com.mzinck.carbn.iap.scanpack.tencredits', 'Starter Pack', 'none', 'apple', 0, 10, 'scanpack'),
    ('com.mzinck.carbn.iap.scanpack.fiftycredits', 'Value Pack', 'none', 'apple', 0, 50, 'scanpack'),
    ('com.mzinck.carbn.iap.scanpack.onehundredcredits', 'Mega Pack', 'none', 'apple', 0, 100, 'scanpack');

-- Create index for subscription products
CREATE INDEX idx_subscription_products_product_id ON subscription_products(product_id);

-- Create table to track scan pack purchases
CREATE TABLE IF NOT EXISTS scan_pack_purchases (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id VARCHAR(255) NOT NULL,
    transaction_id VARCHAR(255) NOT NULL UNIQUE,
    original_transaction_id VARCHAR(255) NOT NULL,
    purchase_date TIMESTAMP NOT NULL,
    environment VARCHAR(20) NOT NULL,
    platform VARCHAR(20) NOT NULL,
    credits_amount INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create indices for scan pack purchases
CREATE INDEX IF NOT EXISTS idx_scan_pack_purchases_user_id ON scan_pack_purchases(user_id);
CREATE INDEX IF NOT EXISTS idx_scan_pack_purchases_transaction_id ON scan_pack_purchases(transaction_id);

-- Create table to track App Store notification history
CREATE TABLE IF NOT EXISTS app_store_notifications (
  id SERIAL PRIMARY KEY,
  notification_uuid VARCHAR(255) UNIQUE NOT NULL,
  notification_type VARCHAR(100) NOT NULL,
  subtype VARCHAR(100),
  user_id INT,
  transaction_id VARCHAR(255),
  original_transaction_id VARCHAR(255),
  product_id VARCHAR(255),
  environment VARCHAR(50),
  signed_date BIGINT,
  processed_at TIMESTAMP DEFAULT NOW(),
  raw_payload JSONB,
  error_message TEXT,  -- Track errors during processing
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

-- Create indices for App Store notifications
CREATE INDEX IF NOT EXISTS idx_app_store_notifications_notification_uuid ON app_store_notifications(notification_uuid);
CREATE INDEX IF NOT EXISTS idx_app_store_notifications_original_transaction_id ON app_store_notifications(original_transaction_id);
CREATE INDEX IF NOT EXISTS idx_app_store_notifications_error_message ON app_store_notifications(error_message) WHERE error_message IS NOT NULL;

-- Create scan history table
CREATE TABLE IF NOT EXISTS scan_history (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    car_id INT NOT NULL,
    color VARCHAR(50) NOT NULL,
    image_path TEXT NOT NULL,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    rejection_reason TEXT,
    scanned_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (car_id) REFERENCES cars (id) ON DELETE CASCADE
);

-- Create indices for scan history lookups
CREATE INDEX IF NOT EXISTS idx_scan_history_user_id ON scan_history(user_id);
CREATE INDEX IF NOT EXISTS idx_scan_history_car_id ON scan_history(car_id);
CREATE INDEX IF NOT EXISTS idx_scan_history_scanned_at ON scan_history(scanned_at);
CREATE INDEX IF NOT EXISTS idx_scan_history_user_car ON scan_history(user_id, car_id, scanned_at);

-- Create the "CarUpgrades" table
CREATE TABLE IF NOT EXISTS car_upgrades (
    id SERIAL PRIMARY KEY,
    user_car_id INT NOT NULL,
    upgrade_type VARCHAR(50) NOT NULL,  -- e.g., 'premium_image', 'performance', 'visual' etc.
    active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB,  -- Store any upgrade-specific data
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_car_id) REFERENCES user_cars (id) ON DELETE CASCADE
);

-- Create indices for car upgrades lookups
CREATE INDEX IF NOT EXISTS idx_car_upgrades_user_car_id ON car_upgrades(user_car_id);
CREATE INDEX IF NOT EXISTS idx_car_upgrades_type ON car_upgrades(upgrade_type);

-- Create a function to automatically update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add trigger for users table
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();