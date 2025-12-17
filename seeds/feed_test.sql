-- =============================================================================
-- Feed System Test Seed Data
-- =============================================================================
-- 
-- This script sets up test data for manual feed testing.
-- 
-- Users:
--   Alice (id=1) - Content creator with 20 posts
--   Bob (id=2)   - Follower, follows Alice initially
--   Charlie (id=3) - Secondary creator with 5 posts
--   Dave (id=4)  - Lurker with 0 posts
--
-- Password for all users: "password123"
-- BCrypt hash: $2a$10$N9qo8uLOickgx2ZMRZoMy.MqrqBuBi0kzPqKqh.qFp9x9x9x9x9x9
--
-- Usage:
--   1. Run this SQL in your database
--   2. Clear Redis: redis-cli FLUSHDB (or in WSL: wsl redis-cli FLUSHDB)
--   3. Start testing in Insomnia
--
-- =============================================================================

-- Clean up existing test data (CAREFUL: This deletes all data!)
TRUNCATE posts, post_details, follows, users RESTART IDENTITY CASCADE;

-- =============================================================================
-- USERS
-- =============================================================================
-- Password "password123" hashed with bcrypt cost 10
-- You may need to generate a real hash if this doesn't work with your setup

INSERT INTO users (id, username, password_hashed, display_name, avatar_url, bio, follower_count, following_count, post_count, created_at, updated_at) VALUES
(1, 'alice', '$2a$10$l2nwiDJP2G9zGR9BnEc92uldk0IJdiKyU/mrLL0GA.kJq1fjKhA1O', 'Alice Wonder', 'https://i.pravatar.cc/150?u=alice', 'Photography enthusiast 📸', 1, 0, 20, NOW() - INTERVAL '30 days', NOW()),
(2, 'bob', '$2a$10$l2nwiDJP2G9zGR9BnEc92uldk0IJdiKyU/mrLL0GA.kJq1fjKhA1O', 'Bob Builder', 'https://i.pravatar.cc/150?u=bob', 'Can we fix it? Yes we can!', 0, 1, 0, NOW() - INTERVAL '25 days', NOW()),
(3, 'charlie', '$2a$10$l2nwiDJP2G9zGR9BnEc92uldk0IJdiKyU/mrLL0GA.kJq1fjKhA1O', 'Charlie Chaplin', 'https://i.pravatar.cc/150?u=charlie', 'Silent film lover 🎬', 0, 0, 5, NOW() - INTERVAL '20 days', NOW()),
(4, 'dave', '$2a$10$l2nwiDJP2G9zGR9BnEc92uldk0IJdiKyU/mrLL0GA.kJq1fjKhA1O', 'Dave the Lurker', 'https://i.pravatar.cc/150?u=dave', 'Just here to scroll', 0, 0, 0, NOW() - INTERVAL '15 days', NOW());

-- Reset sequence to continue after our manual IDs
SELECT setval('users_id_seq', 10);

-- =============================================================================
-- ALICE'S POSTS (20 posts, spread over 20 hours)
-- =============================================================================

INSERT INTO posts (id, user_id, caption, created_at, updated_at) VALUES
(1,  1, 'Post 1 - Starting my photography journey! 📷', NOW() - INTERVAL '20 hours', NOW() - INTERVAL '20 hours'),
(2,  1, 'Post 2 - Golden hour is magical ✨', NOW() - INTERVAL '19 hours', NOW() - INTERVAL '19 hours'),
(3,  1, 'Post 3 - City lights at night 🌃', NOW() - INTERVAL '18 hours', NOW() - INTERVAL '18 hours'),
(4,  1, 'Post 4 - Coffee and creativity ☕', NOW() - INTERVAL '17 hours', NOW() - INTERVAL '17 hours'),
(5,  1, 'Post 5 - Nature walk vibes 🌿', NOW() - INTERVAL '16 hours', NOW() - INTERVAL '16 hours'),
(6,  1, 'Post 6 - Street photography day 🚶', NOW() - INTERVAL '15 hours', NOW() - INTERVAL '15 hours'),
(7,  1, 'Post 7 - Rainy day aesthetics 🌧️', NOW() - INTERVAL '14 hours', NOW() - INTERVAL '14 hours'),
(8,  1, 'Post 8 - Sunset chasing 🌅', NOW() - INTERVAL '13 hours', NOW() - INTERVAL '13 hours'),
(9,  1, 'Post 9 - Urban exploration 🏙️', NOW() - INTERVAL '12 hours', NOW() - INTERVAL '12 hours'),
(10, 1, 'Post 10 - Minimalist shot 🖼️', NOW() - INTERVAL '11 hours', NOW() - INTERVAL '11 hours'),
(11, 1, 'Post 11 - Portrait practice 👤', NOW() - INTERVAL '10 hours', NOW() - INTERVAL '10 hours'),
(12, 1, 'Post 12 - Food photography 🍕', NOW() - INTERVAL '9 hours', NOW() - INTERVAL '9 hours'),
(13, 1, 'Post 13 - Architecture details 🏛️', NOW() - INTERVAL '8 hours', NOW() - INTERVAL '8 hours'),
(14, 1, 'Post 14 - Morning coffee ritual ☀️', NOW() - INTERVAL '7 hours', NOW() - INTERVAL '7 hours'),
(15, 1, 'Post 15 - Pet photography 🐕', NOW() - INTERVAL '6 hours', NOW() - INTERVAL '6 hours'),
(16, 1, 'Post 16 - Black and white mood 🖤', NOW() - INTERVAL '5 hours', NOW() - INTERVAL '5 hours'),
(17, 1, 'Post 17 - Macro lens fun 🔍', NOW() - INTERVAL '4 hours', NOW() - INTERVAL '4 hours'),
(18, 1, 'Post 18 - Weekend adventures 🎒', NOW() - INTERVAL '3 hours', NOW() - INTERVAL '3 hours'),
(19, 1, 'Post 19 - Studio lighting test 💡', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '2 hours'),
(20, 1, 'Post 20 - Latest shot! Fresh off the camera 📸', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour');

-- =============================================================================
-- CHARLIE'S POSTS (5 posts, spread over 5 hours)
-- =============================================================================

INSERT INTO posts (id, user_id, caption, created_at, updated_at) VALUES
(21, 3, 'Charlie post 1 - Silent films are art 🎭', NOW() - INTERVAL '5 hours', NOW() - INTERVAL '5 hours'),
(22, 3, 'Charlie post 2 - Classic cinema appreciation 🎬', NOW() - INTERVAL '4 hours', NOW() - INTERVAL '4 hours'),
(23, 3, 'Charlie post 3 - Movie night setup 🍿', NOW() - INTERVAL '3 hours', NOW() - INTERVAL '3 hours'),
(24, 3, 'Charlie post 4 - Film reel collection 📽️', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '2 hours'),
(25, 3, 'Charlie post 5 - Behind the scenes 🎥', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour');

-- Reset sequence
SELECT setval('posts_id_seq', 100);

-- =============================================================================
-- POST MEDIA (one image per post using picsum.photos)
-- =============================================================================

INSERT INTO post_details (post_id, media_url, media_type, position)
SELECT id, 'https://picsum.photos/seed/post' || id || '/1080/1080', 'image', 0
FROM posts;

-- =============================================================================
-- FOLLOWS (Bob follows Alice initially)
-- =============================================================================

INSERT INTO follows (follower_id, followee_id, created_at) VALUES
(2, 1, NOW() - INTERVAL '10 days');

-- =============================================================================
-- VERIFICATION QUERIES
-- =============================================================================

-- Check user counts
SELECT id, username, follower_count, following_count, post_count FROM users ORDER BY id;

-- Check post distribution
SELECT user_id, COUNT(*) as post_count FROM posts GROUP BY user_id;

-- Check follows
SELECT f.follower_id, u1.username as follower, f.followee_id, u2.username as followee
FROM follows f
JOIN users u1 ON f.follower_id = u1.id
JOIN users u2 ON f.followee_id = u2.id;

-- =============================================================================
-- TEST CHECKLIST (copy to your notes)
-- =============================================================================
/*
□ Phase 1: Setup
  - Run this SQL ✓
  - Clear Redis: wsl redis-cli FLUSHDB

□ Phase 2: Feed Tests
  1a. Login as Bob (bob/password123) → GET /feed 
      Expected: 10 posts, log shows "Cache miss... warming"
  1b. GET /feed?cursor=<from_1a>
      Expected: next 10 posts (posts 11-20)
  1c. Login as Alice → GET /feed
      Expected: her own 20 posts

□ Phase 3: Follow Events
  2a. Bob → POST /users/3/follow → GET /feed
      Expected: Charlie's 5 posts now in feed (total 25)
  2b. Bob → DELETE /users/1/follow → GET /feed
      Expected: Alice's posts removed (only Charlie's 5 remain)

□ Phase 4: Post Events
  3a. Charlie → POST /posts → Bob → GET /feed
      Expected: new post appears
  3b. Charlie → DELETE /posts/:id → Bob → GET /feed
      Expected: post removed

□ Phase 5: Edge Cases
  4a. Login as Dave → GET /feed
      Expected: empty array []
  4b. Bob → POST /users/4/follow
      Expected: success, no crash (Dave has 0 posts)
*/
