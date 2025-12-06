DROP INDEX IF EXISTS idx_users_avatar_key;
ALTER TABLE users DROP COLUMN IF EXISTS avatar_key;
