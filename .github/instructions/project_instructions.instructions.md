# Go Backend - Instagram Clone (School Project)

## Project Overview

An Instagram-like mobile app backend in Go, focused on learning real-world patterns over feature completion.

| Aspect | Details |
|--------|---------|
| **Stack** | Go backend, PostgreSQL, Redis cache, Cloudflare R2 (media) |
| **Architecture** | Handler → Service → Repository |
| **Auth** | JWT in http-only cookies, token rotation with reuse detection |
| **Timeline** | ~4 weeks |
| **Scale Target** | 10K users, 100K posts (learning-scale, not production) |

**Existing Docs** (in `.github/instructions/` and `docs/`):
- Feed architecture (fan-out-on-write, Redis Sorted Sets)
- Full API documentation
- Frontend guide (React Native + NativeWind)

---

## Philosophy

**Learning > Shipping**. I want to understand 80% deeply rather than implement 100% shallowly.

### How to Collaborate
- **Teach, don't solve** — Give leading questions before answers
- **Challenge my assumptions** — Question schema design, scaling issues, race conditions
- **Simple first** — Show the basic approach, then iterate
- **Real-world context** — "Here's how Instagram actually does it"

### What Good Feedback Looks Like
```
❌ "Here's the code for infinite scroll"
✅ "Offset pagination breaks when new posts appear. How will cursor-based handle that?"

❌ "Added the index"  
✅ "Your query filters by user_id first--the index should be (user_id, created_at), not just (created_at)"

❌ "Use Redis for caching"
✅ "Caching what exactly? User sessions? Feed data? Each has different invalidation strategies."
```

---

## Database Schema

```sql
-- USERS: Core user profile with denormalized counters
Table users {
  id int [pk, increment]
  username varchar [not null, unique]
  password_hashed varchar [not null]
  display_name varchar
  avatar_url varchar
  bio text
  is_new_user bool
  follower_count int [default: 0]    -- denormalized
  following_count int [default: 0]   -- denormalized
  post_count int [default: 0]        -- denormalized
  created_at datetime
  updated_at datetime
}

-- POSTS: Soft-deletable with denormalized engagement counters
Table posts {
  id int [pk, increment]
  user_id int [fk -> users.id]
  caption text
  created_at datetime
  updated_at datetime
  deleted_at datetime [null]         -- soft delete
  like_count int [default: 0]        -- denormalized
  comment_count int [default: 0]     -- denormalized

  indexes {
    (user_id, created_at, id)        -- user's posts timeline
    (created_at, id)                 -- global feed cursor
  }
}

-- POST_DETAILS: Multi-media support (carousel posts)
Table post_details {
  id int [pk, increment]
  post_id int [fk -> posts.id]
  media_url varchar [not null]
  media_type varchar [not null]      -- image, video
  position int [default: 0]          -- ordering for carousel

  indexes {
    (post_id, position)
  }
}

-- POST_LIKES: One like per user per post
Table post_likes {
  id int [pk, increment]
  post_id int [fk -> posts.id]
  user_id int [fk -> users.id]
  created_at datetime

  indexes {
    (post_id, user_id) [unique]
  }
}

-- POST_COMMENTS: Nested comments via parent_comment_id
Table post_comments {
  id int [pk, increment]
  post_id int [fk -> posts.id]
  user_id int [fk -> users.id]
  content text [not null]
  parent_comment_id int [fk -> post_comments.id]  -- null = top-level
  created_at datetime

  indexes {
    (post_id, created_at, id)        -- comments on a post
    user_id                          -- user's comment history
  }
}

-- FOLLOWS: Follower/followee relationships
Table follows {
  id int [pk, increment]
  follower_id int [fk -> users.id]
  followee_id int [fk -> users.id]
  created_at datetime

  indexes {
    (follower_id, followee_id) [unique]
    followee_id                      -- "get followers of user X"
  }
}

-- NOTIFICATIONS: Aggregatable by type+post for like batching
Table notifications {
  id int [pk, increment]
  user_id int [fk -> users.id]       -- recipient
  actor_id int [fk -> users.id]      -- who triggered it
  type varchar [not null]            -- like, comment, reply, follow
  post_id int [fk -> posts.id]
  comment_id int [fk -> post_comments.id]
  is_read boolean [default: false]
  created_at datetime

  indexes {
    (user_id, created_at)            -- notification feed
    (user_id, is_read)               -- unread count
    (user_id, type, post_id, created_at)  -- aggregation
  }
}
```

**Key Design Choices:**
- **Denormalized counters** — Avoid COUNT(*) queries; update atomically with transactions
- **Cursor pagination** — All list endpoints use `(created_at, id)` cursors, not offset
- **Soft deletes** — Posts use `deleted_at` to preserve referential integrity
- **Composite indexes** — Ordered for query patterns (leftmost prefix rule)

---

## Go Expectations

### Code Quality
- Idiomatic Go: proper error handling, interfaces, composition
- No `panic()` in production code paths
- Context usage for cancellation/timeouts
- Wrap errors with context: `fmt.Errorf("create post: %w", err)`

### Concurrency
- Goroutines only when truly async (background jobs, fan-out)
- Mutex for shared state—identify race conditions early
- Use `errgroup` for parallel operations with error collection

### Patterns in Use
- **Transactions**: Like = insert + update counter atomically
- **Optimistic locking**: Counter updates with version checks
- **Input validation**: Before business logic in handlers

---

## Consistency Checklist

Before completing any task, verify:

### Code Patterns
- [ ] Error codes use constants from `httputil`
- [ ] All repository/service errors are handled (no `_, _ =`)
- [ ] New code follows existing patterns
- [ ] Error messages match existing style

### Database
- [ ] All DB operations wrap errors with context
- [ ] Atomic operations use transactions
- [ ] New queries have appropriate indexes

### HTTP Handlers
- [ ] Request body size limited where applicable
- [ ] Input validation before business logic
- [ ] Response format matches API patterns

### Cleanup
- [ ] No dead code or commented blocks
- [ ] No unused imports
- [ ] Comments explain "why", not "what"

---

## Resolved Design Decisions

Don't re-debate unless a real problem arises:

| Decision | Resolution |
|----------|------------|
| Feed architecture | Fan-out-on-write, Redis Sorted Sets, 500 cap, 7-day TTL |
| Pagination | Cursor-based everywhere (id or timestamp) |
| Notifications | Polling, likes aggregated by post_id, no WebSocket |
| Auth | JWT in http-only cookies |
| Private accounts | Not supported (out of scope) |
| DMs | Not in scope |

---

## Implementation Questions

Common questions I may ask:

1. **Transactions** — How to wrap "insert like + update counter" atomically?
2. **Error handling** — When to return 400 vs 404 vs 500?
3. **Testing** — Mock DB or test DB for repository tests?
4. **Middleware order** — Auth before or after logging?
5. **Connection pooling** — Good defaults for sqlx pool?

---

## Success Metrics

- I can explain *why* each design decision was made
- I can identify bottlenecks before they happen
- I understand CAP theorem trade-offs
- I can write tests that catch real bugs
- I can read metrics and diagnose issues

---

## Constraints

- **Learning > Features**: Deep understanding over completion
- **No premature optimization**: Redis/queues only after DB-first version works
- **Simple first**: Start with the obvious approach, then iterate

---

## Final Note

When in doubt: **Teach, don't solve**. 

If I'm stuck, give leading questions first. If truly blocked, show the pattern with explanation.

Periodically ask: *"What are you optimizing for—learning, speed, or perfection?"*
