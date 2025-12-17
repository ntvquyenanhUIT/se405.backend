
# Feed Architecture

## API Endpoints (current spec)

> Note: naming/paths should be confirmed (see Open Questions).

- `GET /feed?cursor=<post_id>:<timestamp>`
   - Purpose: home/news feed.
   - Auth: **required** (mobile uses `Authorization: Bearer <access_token>`).
   - Pagination: server-side fixed `limit = 10` (no client `limit` query param).
   - Cursor: `<post_id>:<created_at_unix>`.
   - Response: direct object style `{ posts, next_cursor, has_more }`.

- `GET /users/feed?userid=<id>&cursor=<...>`
   - Purpose: profile grid thumbnails.
   - Auth: public.
   - Pagination: server-side fixed `limit = 10`.
   - Cursor: DB-driven (spec says timestamp; recommendation is a compound cursor to avoid ties; see Open Questions).
   - Response: thumbnails only (media position 0), plus `like_count` and `comment_count`.

- `GET /posts/:id`
   - Purpose: full post detail page.
   - Auth: public **with optional auth** so we can set `has_liked` for logged-in users; otherwise `has_liked=false`.

- `POST /posts`
   - Purpose: create a post.
   - Auth: required.
   - Constraints: 1–10 images; each ≤ 5MB; caption optional.
   - Side effect: fan-out post ID to followers’ feed caches (async via Redis Streams).

- `DELETE /posts/:id`
   - Purpose: delete a post.
   - Auth: required; must be post owner.
   - Side effect: fan-out removal from followers’ feed caches (async via Redis Streams).

## Core Principles

1. **Fan-out-on-write** — When a user posts, push post ID to all followers' cached feeds
2. **Cursor pagination** — Use compound cursor `{id}:{timestamp}` for stable pagination + DB fallback
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
Request: GET /feed?cursor=1040:1732897000
```

**Steps:**
1. Parse cursor: `post_id = 1040`, `timestamp = 1732897000`
2. Get cursor's score: `ZSCORE feed:user:404 1040` → `1732897000` (found!)
3. Get next 10 posts older than cursor: `ZREVRANGEBYSCORE feed:user:404 (1732897000 -inf LIMIT 0 10`
4. Hydrate: Query DB for full post details by IDs
5. Build `next_cursor` from last post: `"{id}:{created_at_unix}"`
6. Refresh TTL: `EXPIRE feed:user:404 604800`
7. Return `{ posts: [...], next_cursor: "1030:1732896000", has_more: true }`

---

### 2. Pull-to-Refresh 🔄

> User pulls down to refresh, wants to see newest posts.

```
Request: GET /feed (no cursor)
```

**Steps:**
1. No cursor in request → start from newest
2. Check cache size: `ZCARD feed:user:404`
3. If cache empty → trigger **Case 11 (Cold Start)** to warm cache first
4. Get top 10 post IDs: `ZREVRANGE feed:user:404 0 9 WITHSCORES`
5. Hydrate: Query DB for full post details by IDs
6. Build `next_cursor` from last post: `"{id}:{score}"`
7. Refresh TTL: `EXPIRE feed:user:404 604800`
8. Return `{ posts: [...], next_cursor: "1041:1732899000", has_more: true }`

---

### 3. Cache Exhausted (DB Fallback) 📉

> User has scrolled through all posts in cache (cache is small or user scrolled a lot).

```
Request: GET /feed?cursor=1045:1732898000 (1045 is the oldest post in cache)
```

**Steps:**
1. Parse cursor: `post_id = 1045`, `timestamp = 1732898000`
2. Get cursor's score: `ZSCORE feed:user:404 1045` → found, but...
3. Query cache: `ZREVRANGEBYSCORE feed:user:404 (1732898000 -inf LIMIT 0 10`
4. If result is empty or insufficient → return it with `has_more=false` so UI can show "you are all caught up!"

---

### 4. Cursor Not Found in Cache ❓

> Cursor post was deleted from cache, but cache is otherwise intact.

```
Request: GET /feed?cursor=902:1732897000 (but 902 is not in cache)
```

**Cursor Format:** `{post_id}:{unix_timestamp}` — encodes both ID and timestamp so we can fall back to DB even if post is deleted.

**Steps:**
1. Parse cursor: `post_id = 902`, `timestamp = 1732897000`
2. Get cursor's score: `ZSCORE feed:user:404 902` → `nil` (not found)
3. **Fall back to DB** using the timestamp from cursor:
   ```sql
   SELECT ... FROM posts
   JOIN follows ON posts.user_id = follows.followee_id
   WHERE follows.follower_id = 404
     AND (posts.created_at, posts.id) < ($timestamp, $post_id)
   ORDER BY posts.created_at DESC, posts.id DESC
   LIMIT 10
   ```
4. Return posts with `next_cursor` pointing to last post
5. **Next request resumes from cache** — the other posts (901, 900, 890...) are still there!

**Why no re-warming needed?**
```
Cache: [950, 940, 930, 920, 910, 902, 901, 900, 890, 880, ...]
                              ↑ deleted
DB returns: [901, 900, 890, 880, 875, 870, 865, 860, 855, 850]
next_cursor = 850:1732896000
Next request: ZSCORE 850 → exists! ✅ Back to normal cache flow
```

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

> User hasn't followed anyone yet.

**On fan-out-on-write:** No followers = no targets = nothing to do.

**On read:**
1. Cache is empty → triggers Case 11 (Cold Start)
2. Cold Start queries DB but returns 0 posts (no followees)
3. Return `{ posts: [], next_cursor: null, has_more: false }`
4. UI displays: *"Follow some accounts to see posts!"*

---

### 11. Cold Start (Cache Warming) 🔥

> Cache is empty. Applies to: new users after onboarding, or returning users inactive > 7 days.

```
Trigger: ZCARD feed:user:404 == 0 during GET /feed
```

**Steps:**
1. Query DB for recent posts from all followed users:
   ```sql
   SELECT id, user_id, caption, created_at, like_count, comment_count, ...
   FROM posts
   JOIN follows ON posts.user_id = follows.followee_id
   WHERE follows.follower_id = 404 AND posts.deleted_at IS NULL
   ORDER BY posts.created_at DESC
   LIMIT 500
   ```
2. If DB returns 0 posts → return empty feed (Case 10 behavior)
3. Warm cache with all 500 posts (pipeline for efficiency):
   ```
   ZADD feed:user:404 <created_at_1> <post_id_1>
   ZADD feed:user:404 <created_at_2> <post_id_2>
   ... (pipelined)
   EXPIRE feed:user:404 604800
   ```
4. First 10 posts are already hydrated (full details from step 1)
5. Build `next_cursor` from 10th post: `"{id}:{created_at_unix}"`
6. Return `{ posts: [first 10], next_cursor: "...", has_more: true }`

**Frontend UX:**

| Scenario | How to detect | UX |
|----------|---------------|----|
| New user after onboarding | `is_new_user = true` | Show "Preparing your feed..." spinner |
| Returning inactive user | `is_new_user = false` | Normal feed loading spinner |

**Note:** Backend logic is identical for both. Only frontend presentation differs.

---

## Cursor Format

**Format:** `{post_id}:{unix_timestamp}`

**Example:** `1041:1732899000`

**Why compound cursor?**
- If cursor post is deleted, we still have the timestamp for DB fallback
- No extra DB lookup needed to find post's `created_at`
- Self-contained — cursor has all info needed for pagination

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
  "next_cursor": "1041:1732899000",
  "has_more": true
}
```

| Field | Description |
|-------|-------------|
| `posts` | Array of hydrated post objects |
| `next_cursor` | Compound cursor `{id}:{timestamp}` for next request |
| `has_more` | `false` when user has seen all available posts |

---

## Open Questions (need to settle before implementation)

1. **Cursor for `GET /users/feed`:** spec originally said timestamp-only.
   - Recommendation (accepted): use a compound cursor like `<post_id>:<created_at_unix>` (same style as `/feed`) to avoid ties.

2. **DB prerequisites:** `posts` tables exist in DB but migration files were missing.
   - This repo now includes `000006_create_posts_tables.*.sql` and `000007_create_post_comments_table.*.sql`.
