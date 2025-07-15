-- Migration to add shared_cars table

-- Create shared_cars table to store information about shared car links
CREATE TABLE IF NOT EXISTS shared_cars (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    user_car_id INTEGER NOT NULL REFERENCES user_cars(id),
    token VARCHAR(64) NOT NULL UNIQUE,
    view_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    UNIQUE(user_id, user_car_id)
);

-- Create index on token for faster lookups
CREATE INDEX IF NOT EXISTS idx_shared_cars_token ON shared_cars(token);

-- Create index on expiration date for cleanup
CREATE INDEX IF NOT EXISTS idx_shared_cars_expires_at ON shared_cars(expires_at);