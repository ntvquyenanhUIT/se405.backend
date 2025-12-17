-- Recreated migration (version 6) to match the existing DB schema.

CREATE TABLE posts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    caption TEXT,
    like_count INT DEFAULT 0,
    comment_count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- Feed/profile queries: ignore soft-deleted posts
CREATE INDEX idx_posts_user_id ON posts(user_id, created_at DESC) WHERE (deleted_at IS NULL);
CREATE INDEX idx_posts_created_at ON posts(created_at DESC) WHERE (deleted_at IS NULL);

CREATE TABLE post_details (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    media_url VARCHAR NOT NULL,
    media_type VARCHAR NOT NULL,
    "position" INT DEFAULT 0
);

CREATE INDEX idx_post_details_post_id ON post_details(post_id, "position");

CREATE TABLE post_likes (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (post_id, user_id)
);

CREATE INDEX idx_post_likes_user_post ON post_likes(user_id, post_id);
