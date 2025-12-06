ALTER TABLE users
ADD COLUMN avatar_key VARCHAR(255);

CREATE INDEX idx_users_avatar_key ON users(avatar_key);
