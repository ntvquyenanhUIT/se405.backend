CREATE TABLE post_comments (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    parent_comment_id BIGINT REFERENCES post_comments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Comments on a post (cursor-friendly)
CREATE INDEX idx_post_comments_post_created ON post_comments(post_id, created_at DESC, id DESC);

-- User's comment history
CREATE INDEX idx_post_comments_user ON post_comments(user_id, created_at DESC);
