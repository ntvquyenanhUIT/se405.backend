-- Make display_name NOT NULL
-- Update any existing NULL values to username as fallback
ALTER TABLE users ALTER COLUMN display_name SET NOT NULL;

-- Make avatar_url NOT NULL
-- IMPORTANT: You must set DEFAULT_AVATAR_URL in your .env before running this migration
-- Update existing NULL values manually first if needed:
-- UPDATE users SET avatar_url = 'your_default_avatar_url' WHERE avatar_url IS NULL;
ALTER TABLE users ALTER COLUMN avatar_url SET NOT NULL;

-- Make avatar_key NOT NULL with default empty string
-- avatar_key can be empty for default avatar (no key in R2)
UPDATE users SET avatar_key = '' WHERE avatar_key IS NULL;
ALTER TABLE users ALTER COLUMN avatar_key SET NOT NULL;
ALTER TABLE users ALTER COLUMN avatar_key SET DEFAULT '';
