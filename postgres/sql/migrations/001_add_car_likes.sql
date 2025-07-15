-- Add support for car likes in the likes table
-- Step 1: Rename existing feed_item_id column to target_id
ALTER TABLE likes RENAME COLUMN feed_item_id TO target_id;

-- Step 2: Add a target_type column to distinguish between feed items and user cars
ALTER TABLE likes ADD COLUMN target_type VARCHAR(20) NOT NULL DEFAULT 'feed_item';

-- Step 3: Drop the existing foreign key constraint
ALTER TABLE likes DROP CONSTRAINT likes_feed_item_id_fkey;

-- Step 4: Drop the unique constraint on user_id and feed_item_id (now target_id)
ALTER TABLE likes DROP CONSTRAINT likes_user_id_feed_item_id_key;

-- Step 5: Add a new unique constraint on user_id, target_id, and target_type
ALTER TABLE likes ADD CONSTRAINT likes_user_target_unique UNIQUE (user_id, target_id, target_type);

-- Step 6: Update indices
DROP INDEX IF EXISTS idx_likes_feed_item_id;
CREATE INDEX idx_likes_target_id ON likes(target_id);
CREATE INDEX idx_likes_target_type ON likes(target_type);

-- Step 7: Add likes_count column to user_cars table
ALTER TABLE user_cars ADD COLUMN likes_count INT NOT NULL DEFAULT 0;

-- Step 8: Create a trigger function to update the likes_count in user_cars table
CREATE OR REPLACE FUNCTION update_user_car_likes_count() RETURNS TRIGGER AS $$
BEGIN
    IF (TG_OP = 'INSERT' AND NEW.target_type = 'user_car') THEN
        UPDATE user_cars SET likes_count = likes_count + 1 WHERE id = NEW.target_id;
    ELSIF (TG_OP = 'DELETE' AND OLD.target_type = 'user_car') THEN
        UPDATE user_cars SET likes_count = likes_count - 1 WHERE id = OLD.target_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Step 9: Create triggers to update likes_count when likes are added or removed
CREATE TRIGGER update_user_car_likes_count_insert
    AFTER INSERT ON likes
    FOR EACH ROW
    EXECUTE FUNCTION update_user_car_likes_count();

CREATE TRIGGER update_user_car_likes_count_delete
    AFTER DELETE ON likes
    FOR EACH ROW
    EXECUTE FUNCTION update_user_car_likes_count();

-- Step 10: Add a function to get likes count for a user car
CREATE OR REPLACE FUNCTION get_user_car_likes_count(p_user_car_id INT) RETURNS INT AS $$
DECLARE
    likes_count INT;
BEGIN
    SELECT COUNT(*) INTO likes_count
    FROM likes
    WHERE target_id = p_user_car_id AND target_type = 'user_car';
    
    RETURN likes_count;
END;
$$ LANGUAGE plpgsql;

-- Step 11: Update existing likes_count for user_cars based on current data
UPDATE user_cars uc
SET likes_count = (
    SELECT COUNT(*)
    FROM likes l
    WHERE l.target_id = uc.id AND l.target_type = 'user_car'
);