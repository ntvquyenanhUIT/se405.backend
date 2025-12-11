
# Feed Architecture

## Core Principles

1. **Fan-out-on-write** — When a user posts, push post ID to all followers' cached feeds
2. **Cursor pagination** — Use `post_id` as cursor (not index) for stable pagination
3. **Chronological order** — Feed sorted by `created_at DESC` (newest first)
4. **Cache configuration** — Cap: 500 items | TTL: 7 days (refresh on every access)
5. **Cold start** — Precompute feed for new users (enforce following some accounts first sign-in)

---

## Cache Data Structure: Redis Sorted Set

We use **Sorted Set** instead of List for efficient operations:

```
Key:    feed:user:404
Value:  Sorted Set { post_id: score (timestamp) }

Example:
  feed:user:404 = {
    1050: 1732900000,   ← newest (highest score)
    1048: 1732899000,
    1045: 1732898000,
    1040: 1732897000,   ← oldest (lowest score)
  }
```

**Why Sorted Set over List?**

| Operation | List | Sorted Set |
|-----------|------|------------|
| Insert in sorted order | ❌ LPUSH breaks order | ✅ ZADD auto-sorts |
| Delete by post ID | ❌ O(n) LREM | ✅ O(log n) ZREM |
| Cursor lookup | ❌ O(n) scan | ✅ O(log n) ZSCORE |
| Range query | ✅ LRANGE | ✅ ZREVRANGEBYSCORE |

### Key Redis Commands

| Operation | Command |
|-----------|---------|
| Add post | `ZADD feed:user:404 <timestamp> <post_id>` |
| Maintain cap | `ZREMRANGEBYRANK feed:user:404 0 -501` (keep top 500) |
| Get newest N | `ZREVRANGE feed:user:404 0 <N-1>` |
| Get cursor score | `ZSCORE feed:user:404 <post_id>` |
| Get posts older than cursor | `ZREVRANGEBYSCORE feed:user:404 (<cursor_score> -inf LIMIT 0 10` |
| Delete post | `ZREM feed:user:404 <post_id>` |
| Refresh TTL | `EXPIRE feed:user:404 604800` |

---

## Workflows

### 1. Normal Scroll (Cache Hit) ✅

> User scrolls down, cursor points to a post that exists in cache.

```
Request: GET /feed?cursor=1040
```

**Steps:**
1. Parse cursor from request (`cursor = 1040`)
2. Get cursor's score: `ZSCORE feed:user:404 1040` → `1732897000`
3. Get next 10 posts older than cursor: `ZREVRANGEBYSCORE feed:user:404 (1732897000 -inf LIMIT 0 10`
4. Hydrate: Query DB for full post details by IDs
5. Build `next_cursor` = last post ID in response
6. Refresh TTL: `EXPIRE feed:user:404 604800`
7. Return `{ posts: [...], next_cursor: "1030", has_more: true }`

---

### 2. Pull-to-Refresh 🔄

> User pulls down to refresh, wants to see newest posts.

```
Request: GET /feed (no cursor)
```

**Steps:**
1. No cursor in request → start from newest
2. Get top 10 post IDs: `ZREVRANGE feed:user:404 0 9`
3. If cache empty → trigger cold start flow (populate cache from DB first)
4. Hydrate: Query DB for full post details by IDs
5. Build `next_cursor` = last post ID in response
6. Refresh TTL: `EXPIRE feed:user:404 604800`
7. Return `{ posts: [...], next_cursor: "1041", has_more: true }`

---

### 3. Cache Exhausted (DB Fallback) 📉

> User has scrolled through all posts in cache (cache is small or user scrolled a lot).

```
Request: GET /feed?cursor=1045 (1045 is the oldest post in cache)
```

**Steps:**
1. Parse cursor from request (`cursor = 1045`)
2. Get cursor's score: `ZSCORE feed:user:404 1045`
3. Query cache: `ZREVRANGEBYSCORE feed:user:404 (<score> -inf LIMIT 0 10`
4. If result is empty or insufficient → **Fall back to DB**:
   - Join `posts` with `follows` table
   - Filter: `follower_id = current_user` AND `(created_at, id) < cursor's values`
   - Order by `created_at DESC, id DESC`
   - Limit 10
5. If DB returns posts → Return them with `has_more: true`
6. If DB returns empty → Return `{ posts: [], next_cursor: null, has_more: false }`
7. UI displays: *"You're all caught up!"*

**Note:** Do NOT re-warm cache with old posts. Cache is for recent posts only.

---

### 4. Cursor Not Found in Cache ❓

> Cursor points to a post that no longer exists in cache (TTL expired, post deleted, etc.)

```
Request: GET /feed?cursor=1040 (but 1040 is not in cache)
```

**Steps:**
1. Parse cursor from request (`cursor = 1040`)
2. Get cursor's score: `ZSCORE feed:user:404 1040` → `nil` (not found)
3. **Fall back to DB**: Same as Case 3 (Cache Exhausted)
   - Query posts older than cursor from followed users
4. If cursor post doesn't exist in DB either:
   - Option: Return from top (treat as refresh) for better UX
5. Return response with appropriate `has_more` flag

---

### 5. User Follows Someone New ➕

> User follows a new account, their recent posts should appear in feed.

```
Event: User 404 follows User 501
```

**Steps:**
1. Query DB for followee's recent posts:
   - `SELECT id, created_at FROM posts WHERE user_id = 501 ORDER BY created_at DESC LIMIT 5`
2. Add each post to cache (auto-sorted by timestamp):
   - `ZADD feed:user:404 <created_at_timestamp> <post_id>` for each post
3. Maintain cache cap: `ZREMRANGEBYRANK feed:user:404 0 -501`
4. Future posts from 501 arrive via normal fan-out-on-write

**Note:** Posts are auto-sorted by score (timestamp), no order issues!

---

### 6. User Unfollows Someone ➖

> User unfollows an account, their posts should be removed from feed.

```
Event: User 404 unfollows User 501
```

**Steps:**
1. Query DB for unfollowed user's recent posts (within TTL window):
   - `SELECT id FROM posts WHERE user_id = 501 AND created_at > NOW() - INTERVAL '7 days'`
2. Remove each post from cache:
   - `ZREM feed:user:404 <post_id>` for each post (O(log n) per removal)
3. Cache is now clean of unfollowed user's posts

**Performance:** Even with 100 posts to remove, 100 × O(log 500) is acceptable.

---

### 7. Post Deleted 🗑️

> User deletes their own post, should be removed from all followers' feeds.

```
Event: User 501 deletes post 1050
```

**Steps:**
1. Soft delete in DB: `UPDATE posts SET deleted_at = NOW() WHERE id = 1050`
2. Get all followers of post author: `SELECT follower_id FROM follows WHERE followee_id = 501`
3. Send to message queue: `{ action: "remove_post", post_id: 1050, follower_ids: [...] }`
4. Background worker removes from each follower's cache:
   - `ZREM feed:user:<follower_id> 1050` for each follower

**Note:** Same fan-out pattern as posting, but removes instead of adds.

---

### 8. Post Edited ✏️

> User edits their post caption.

**No action needed!** Cache only stores `post_id`, not content. Hydration fetches latest data from DB.

---

### 9. Redis Down (Graceful Degradation) 🔥

> Redis is unavailable, app should still function.

**Steps:**
1. Catch Redis connection error
2. Fall back to DB query (same as Cache Exhausted case):
   - Join `posts` with `follows` table
   - Filter by `follower_id = current_user`
   - Order by `created_at DESC, id DESC`
   - Limit 10
3. Return posts normally (just slower)
4. Log error for monitoring/alerting

**Principle:** Cache is optimization, not requirement. DB is source of truth.

---

### 10. User with 0 Followees 👤

> New user hasn't followed anyone yet (after onboarding).

**On fan-out-on-write:** No followers = no targets = nothing to do.

**On read:**
1. Cache is empty
2. DB query returns no posts (no followees)
3. Return `{ posts: [], next_cursor: null, has_more: false }`
4. UI displays: *"Follow some accounts to see posts!"*

---

## Response Format

```json
{
  "posts": [
    {
      "id": 1050,
      "user_id": 501,
      "caption": "...",
      "created_at": "2025-11-29T10:00:00Z",
      "like_count": 42,
      "comment_count": 5,
      "media": [...]
    }
  ],
  "next_cursor": "1041",
  "has_more": true
}
```

| Field | Description |
|-------|-------------|
| `posts` | Array of hydrated post objects |
| `next_cursor` | Last post ID in response (use for next request) |
| `has_more` | `false` when user has seen all available posts |

