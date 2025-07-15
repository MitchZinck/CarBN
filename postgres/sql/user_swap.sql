BEGIN;

-- Create separate temporary tables for each user to avoid UNION ALL column naming issues
CREATE TEMPORARY TABLE user_4_data AS 
SELECT 
    email,
    auth_provider,
    auth_provider_id
FROM users WHERE id = 4;

CREATE TEMPORARY TABLE user_151_data AS 
SELECT 
    email,
    auth_provider,
    auth_provider_id
FROM users WHERE id = 151;

-- First update user 4 with temporary values that won't conflict
UPDATE users 
SET 
    email = email || '_temp_swap',
    auth_provider_id = auth_provider_id || '_temp_swap'
WHERE id = 4;

-- Now update user 151 with user 4's original values
UPDATE users 
SET 
    email = (SELECT email FROM user_4_data),
    auth_provider = (SELECT auth_provider FROM user_4_data),
    auth_provider_id = (SELECT auth_provider_id FROM user_4_data)
WHERE id = 151;

-- Finally, update user 4 with user 151's original values
UPDATE users 
SET 
    email = (SELECT email FROM user_151_data),
    auth_provider = (SELECT auth_provider FROM user_151_data),
    auth_provider_id = (SELECT auth_provider_id FROM user_151_data)
WHERE id = 4;

DROP TABLE user_4_data;
DROP TABLE user_151_data;
COMMIT;