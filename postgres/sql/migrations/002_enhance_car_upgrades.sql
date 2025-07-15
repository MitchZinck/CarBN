-- Add additional fields to car_upgrades metadata for timestamp tracking
-- This migration doesn't alter the table structure but documents the expected metadata fields

-- car_upgrades.metadata should include the following fields:
-- * premium_high_res: Path to premium high resolution image
-- * premium_low_res: Path to premium low resolution image
-- * original_high_res: Path to original high resolution image (for reverting)
-- * original_low_res: Path to original low resolution image (for reverting)
-- * background_index: Index of the premium background used
-- * timestamp: Unix timestamp in milliseconds used in the filename for cache busting

-- Create comment on table
COMMENT ON TABLE car_upgrades IS 'Tracks upgrades applied to user cars including premium images';

-- Create comments on JSON metadata fields
COMMENT ON COLUMN car_upgrades.metadata IS 'JSON metadata for the upgrade. For premium_image upgrades, includes original and premium image paths, background index, and timestamp.';