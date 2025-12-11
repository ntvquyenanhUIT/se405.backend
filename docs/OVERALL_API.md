

# API Documentation

## A. Feed APIs

> APIs for the main news feed experience

---

### 1. Get Feed (Open App / Refresh / Scroll)

```
GET /feed
GET /feed?cursor=<post_id>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Query params:
  - `cursor` (optional): Last post ID from previous response
  - `limit` (optional): Number of posts, default 10, max 50

**Response:** `200 OK`
```json
{
  "data": {
    "posts": [
      {
        "id": 1050,
        "user": {
          "id": 501,
          "username": "johndoe",
          "display_name": "John Doe",
          "avatar_url": "https://..."
        },
        "caption": "Beautiful sunset!",
        "created_at": "2025-12-02T10:00:00Z",
        "like_count": 42,
        "comment_count": 5,
        "is_liked": true,
        "media": [
          {
            "url": "https://...",
            "type": "image",
            "position": 0
          }
        ]
      }
    ]
  },
  "meta": {
    "count": 10,
    "has_more": true,
    "next_cursor": "1040"
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT

**Side Effects:**
- Refresh cache TTL
- Cold start: Populate cache from DB if empty

---

## B. Post Interaction APIs

> APIs for interacting with individual posts

---

### 1. Like a Post

```
POST /posts/:id/like
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — Post ID

**Response:** `201 Created`
```json
{
  "data": {
    "like_count": 43
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — Post doesn't exist
- `409 Conflict` — Already liked

**Side Effects:**
- Increment `like_count` in posts table
- Create notification for post author (async)

---

### 2. Unlike a Post

```
DELETE /posts/:id/like
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — Post ID

**Response:** `200 OK`
```json
{
  "data": {
    "like_count": 42
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — Post doesn't exist or not liked

**Side Effects:**
- Decrement `like_count` in posts table

---

### 3. Create a Comment

```
POST /posts/:id/comments
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — Post ID
- Body:
```json
{
  "content": "Great photo!",
  "parent_comment_id": null
}
```

**Response:** `201 Created`
```json
{
  "data": {
    "comment": {
      "id": 5001,
      "content": "Great photo!",
      "user": {
        "id": 404,
        "username": "janedoe",
        "display_name": "Jane Doe",
        "avatar_url": "https://..."
      },
      "created_at": "2025-12-02T10:30:00Z",
      "parent_comment_id": null
    }
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — Post doesn't exist
- `400 Bad Request` — Empty content or invalid parent_comment_id

**Side Effects:**
- Increment `comment_count` in posts table
- Create notification for post author (async)
- If reply: Create notification for parent comment author (async)

---

### 4. Get Comments for a Post

```
GET /posts/:id/comments?cursor=<comment_id>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — Post ID
- Query params:
  - `cursor` (optional): Last comment ID
  - `limit` (optional): Default 20, max 50

**Response:** `200 OK`
```json
{
  "data": {
    "comments": [
      {
        "id": 5001,
        "content": "Great photo!",
        "user": {
          "id": 404,
          "username": "janedoe",
          "display_name": "Jane Doe",
          "avatar_url": "https://..."
        },
        "created_at": "2025-12-02T10:30:00Z",
        "parent_comment_id": null,
        "reply_count": 3
      }
    ]
  },
  "meta": {
    "count": 20,
    "has_more": true,
    "next_cursor": "4980"
  }
}
```

---

### 5. Delete a Comment

```
DELETE /posts/:postId/comments/:commentId
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `postId` — Post ID, `commentId` — Comment ID

**Response:** `200 OK`
```json
{
  "success": true
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `403 Forbidden` — Not your comment
- `404 Not Found` — Comment doesn't exist

**Side Effects:**
- Decrement `comment_count` in posts table

---

## C. Follow APIs

> APIs for following/unfollowing users

---

### 1. Follow a User

```
POST /users/:id/follow
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID to follow

**Response:** `201 Created`
```json
{
  "data": {
    "is_following": true,
    "follower_count": 1001
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — User doesn't exist
- `409 Conflict` — Already following
- `400 Bad Request` — Cannot follow yourself

**Side Effects:**
- Add 5 recent posts from followee to follower's feed cache (ZADD)
- Create notification for followee (async)

---

### 2. Unfollow a User

```
DELETE /users/:id/follow
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID to unfollow

**Response:** `200 OK`
```json
{
  "data": {
    "is_following": false,
    "follower_count": 1000
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — User doesn't exist or not following

**Side Effects:**
- Remove followee's posts from follower's feed cache (ZREM)

---

### 3. Check Follow Status

```
GET /users/:id/follow
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID to check

**Response:** `200 OK`
```json
{
  "data": {
    "is_following": true
  }
}
```

---

## D. User Profile APIs

> APIs for viewing user profiles and their content

---

### 1. Search Users

```
GET /users/search?q=<query>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Query params:
  - `q` (required): Search query (min 1 character)
  - `limit` (optional): Default 10, max 20

**Response:** `200 OK`
```json
{
  "data": {
    "users": [
      {
        "id": 501,
        "username": "johndoe",
        "display_name": "John Doe",
        "avatar_url": "https://..."
      },
      {
        "id": 502,
        "username": "johnsmith",
        "display_name": "John Smith",
        "avatar_url": "https://..."
      }
    ]
  },
  "meta": {
    "count": 2
  }
}
```

**Search Behavior:**
- Case-insensitive prefix match on `username`
- Returns lightweight user objects (no bio, no counts)
- No pagination — limited results for autocomplete UX

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `400 Bad Request` — Missing or empty `q` parameter

**SQL Query:**
```sql
SELECT id, username, display_name, avatar_url
FROM users
WHERE username ILIKE $1 || '%'
ORDER BY username
LIMIT $2;
```

---

### 2. Get User Profile

```
GET /users/:id
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID

**Response:** `200 OK`
```json
{
  "data": {
    "user": {
      "id": 501,
      "username": "johndoe",
      "display_name": "John Doe",
      "avatar_url": "https://...",
      "bio": "Photography enthusiast",
      "follower_count": 1000,
      "following_count": 250,
      "post_count": 42,
      "is_following": true
    }
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — User doesn't exist

---

### 2. Get User's Posts

```
GET /users/:id/posts?cursor=<post_id>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID
- Query params:
  - `cursor` (optional): Last post ID
  - `limit` (optional): Default 12, max 50

**Response:** `200 OK`
```json
{
  "data": {
    "posts": [
      {
        "id": 1050,
        "thumbnail_url": "https://...",
        "like_count": 42,
        "comment_count": 5,
        "media_count": 3
      }
    ]
  },
  "meta": {
    "count": 12,
    "has_more": true,
    "next_cursor": "1020"
  }
}
```

**Note:** Returns lightweight post objects (thumbnails) for grid view. Use `GET /posts/:id` for full post details.

---

### 3. Get User's Followers

```
GET /users/:id/followers?cursor=<user_id>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID
- Query params:
  - `cursor` (optional): Last user ID
  - `limit` (optional): Default 20, max 50

**Response:** `200 OK`
```json
{
  "data": {
    "users": [
      {
        "id": 404,
        "username": "janedoe",
        "display_name": "Jane Doe",
        "avatar_url": "https://...",
        "is_following": false
      }
    ]
  },
  "meta": {
    "count": 20,
    "has_more": true,
    "next_cursor": "380"
  }
}
```

---

### 4. Get User's Following

```
GET /users/:id/following?cursor=<user_id>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — User ID
- Query params:
  - `cursor` (optional): Last user ID
  - `limit` (optional): Default 20, max 50

**Response:** `200 OK`
```json
{
  "data": {
    "users": [
      {
        "id": 501,
        "username": "johndoe",
        "display_name": "John Doe",
        "avatar_url": "https://...",
        "is_following": true
      }
    ]
  },
  "meta": {
    "count": 20,
    "has_more": true,
    "next_cursor": "480"
  }
}
```

---

## E. Single Post API

> API for viewing a single post's full details

---

### 1. Get Post Details

```
GET /posts/:id
```

**Request:**
- Cookie: `jwt` (http-only)
- Path: `id` — Post ID

**Response:** `200 OK`
```json
{
  "data": {
    "post": {
      "id": 1050,
      "user": {
        "id": 501,
        "username": "johndoe",
        "display_name": "John Doe",
        "avatar_url": "https://..."
      },
      "caption": "Beautiful sunset!",
      "created_at": "2025-12-02T10:00:00Z",
      "like_count": 42,
      "comment_count": 5,
      "is_liked": true,
      "media": [
        {
          "url": "https://...",
          "type": "image",
          "position": 0
        }
      ]
    }
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `404 Not Found` — Post doesn't exist or deleted

---

## F. Notification APIs

> APIs for notification management (polling-based, no WebSocket)

---

### 1. Get Notifications

```
GET /notifications?cursor=<timestamp>&limit=<n>
```

**Request:**
- Cookie: `jwt` (http-only)
- Query params:
  - `cursor` (optional): Unix timestamp of last notification group's `latest_at`
  - `limit` (optional): Default 20, max 50

**Response:** `200 OK`
```json
{
  "data": {
    "notifications": [
      {
        "type": "like",
        "post_id": 1050,
        "post_thumbnail_url": "https://...",
        "actors": [
          {
            "id": 404,
            "username": "janedoe",
            "avatar_url": "https://..."
          },
          {
            "id": 405,
            "username": "bobsmith",
            "avatar_url": "https://..."
          }
        ],
        "actor_count": 5,
        "latest_at": "2025-12-03T10:00:00Z",
        "is_read": false
      },
      {
        "type": "comment",
        "post_id": 1050,
        "post_thumbnail_url": "https://...",
        "comment_preview": "Great photo!",
        "actor": {
          "id": 404,
          "username": "janedoe",
          "avatar_url": "https://..."
        },
        "created_at": "2025-12-03T09:30:00Z",
        "is_read": true
      },
      {
        "type": "follow",
        "actor": {
          "id": 406,
          "username": "newuser",
          "avatar_url": "https://..."
        },
        "created_at": "2025-12-03T09:00:00Z",
        "is_read": true
      }
    ]
  },
  "meta": {
    "count": 20,
    "has_more": true,
    "next_cursor": "1733216400",
    "unread_count": 3
  }
}
```

**Aggregation Rules:**
- `like`: Grouped by `post_id`, shows up to 2 actors + count
- `comment`, `reply`, `follow`: Not aggregated, shown individually

**Cursor:** Unix timestamp of the last item's `latest_at` (for likes) or `created_at` (for others)

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT

---

### 2. Mark Notifications as Read

```
PATCH /notifications/read
```

**Request:**
- Cookie: `jwt` (http-only)
- Body:
```json
{
  "type": "like",
  "post_id": 1050
}
```

Or for non-aggregated notifications:
```json
{
  "notification_ids": [101, 102, 103]
}
```

Or mark all as read:
```json
{
  "all": true
}
```

**Response:** `200 OK`
```json
{
  "data": {
    "marked_count": 5
  }
}
```

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT
- `400 Bad Request` — Invalid body format

---

### 3. Get Unread Count

```
GET /notifications/unread-count
```

**Request:**
- Cookie: `jwt` (http-only)

**Response:** `200 OK`
```json
{
  "data": {
    "unread_count": 12
  }
}
```

**Use Case:** Badge count on notification icon. Poll this endpoint periodically (e.g., every 30s).

**Errors:**
- `401 Unauthorized` — Invalid/missing JWT

---

### Notification Types Reference

| Type | Trigger | Aggregated | Target |
|------|---------|------------|--------|
| `like` | Someone likes your post | ✅ By post | Post author |
| `comment` | Someone comments on your post | ❌ | Post author |
| `reply` | Someone replies to your comment | ❌ | Comment author |
| `follow` | Someone follows you | ❌ | Followed user |

---

### Database Schema

```sql
CREATE TABLE notifications (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),     -- recipient
  actor_id BIGINT NOT NULL REFERENCES users(id),    -- who triggered
  type VARCHAR(20) NOT NULL,                         -- like, comment, reply, follow
  post_id BIGINT REFERENCES posts(id),              -- nullable (follow has no post)
  comment_id BIGINT REFERENCES post_comments(id),   -- for reply type
  is_read BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for fetching user's notifications
CREATE INDEX idx_notifications_user_created 
  ON notifications(user_id, created_at DESC);

-- Index for aggregating likes by post
CREATE INDEX idx_notifications_like_aggregation 
  ON notifications(user_id, type, post_id, created_at DESC) 
  WHERE type = 'like';

-- Index for unread count
CREATE INDEX idx_notifications_unread 
  ON notifications(user_id, is_read) 
  WHERE is_read = FALSE;
```

---

## Error Response Format

All errors follow this format:

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "Post not found"
  }
}
```

| HTTP Status | Code | When |
|-------------|------|------|
| 400 | `BAD_REQUEST` | Invalid input, missing fields |
| 401 | `UNAUTHORIZED` | Missing/invalid JWT |
| 403 | `FORBIDDEN` | Action not allowed |
| 404 | `NOT_FOUND` | Resource doesn't exist |
| 409 | `CONFLICT` | Duplicate action (already liked, already following) |
| 500 | `INTERNAL_ERROR` | Server error |
