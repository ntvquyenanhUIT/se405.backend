-- Revert avatar_key to nullable
ALTER TABLE users ALTER COLUMN avatar_key DROP DEFAULT;
ALTER TABLE users ALTER COLUMN avatar_key DROP NOT NULL;

-- Revert avatar_url to nullable
ALTER TABLE users ALTER COLUMN avatar_url DROP NOT NULL;

-- Revert display_name to nullable
ALTER TABLE users ALTER COLUMN display_name DROP NOT NULL;
