# Redis & Message Queue Implementation Plan

> Goal: build the full Redis-first system, but **ship in small stages** (pause after each layer so it stays learnable).

## Decisions Made

### Why Redis Streams?

| Concern | Go Channels | Redis Streams |
|---------|-------------|---------------|
| Durability | ❌ Lost on crash | ✅ Persisted |
| Multi-worker | ❌ Manual coordination | ✅ Consumer groups |
| Retry support | ❌ Build from scratch | ✅ PEL + XCLAIM |

**Decision:** Use Redis Streams to avoid lost tasks on process crash.

Reference: Redis Streams + consumer groups (`XGROUP`, `XREADGROUP`, `XACK`, `XPENDING`, `XAUTOCLAIM`).

---

### Stream Architecture

**Decision:** Separate streams for feed vs notifications (different workload intensity).

| Stream | Events | Concurrency |
|--------|--------|-------------|
| `stream:feed` | post.created, post.deleted, user.followed, user.unfollowed | 5 workers |
| `stream:notification` | post.liked, post.commented, comment.replied, user.followed | 2 workers |

**Note:** `user.followed` publishes to BOTH streams (cache update + notification).

Implementation note: a single stream can have multiple consumer groups. Entries are appended once and delivered independently to each consumer group.

---

### Cache Operations

| Trigger | Sync/Async | Operation |
|---------|------------|-----------|
| New post created | Async | Add post to all followers' caches |
| Cold start (empty cache) | Sync | Warm cache with 500 posts from DB |
| Follow user | Async | Add followee's recent posts to cache |
| Unfollow user | Async | Remove followee's posts from cache |
| Delete post | Async | Remove post from all followers' caches |

---

### Retry Strategy

**Decision:** Option A — On error, leave message in PEL for retry via periodic reclaim loop.

- Reclaim interval: 30 seconds
- Idle threshold: 1 minute
- Max retries: 3 (then move to dead-letter stream)

Concrete commands behind this design:
- Read new messages: `XREADGROUP ... STREAMS <stream> >`
- Ack on success: `XACK <stream> <group> <id>`
- Inspect retries: `XPENDING` / `XPENDINGEXT`
- Reclaim stuck: `XAUTOCLAIM <stream> <group> <consumer> <min-idle> 0-0 COUNT <n>`

---

### Redis Wrapper Philosophy

**Decision:** Only wrap multi-command operations; use raw client for simple ops.

Wrapped operations (FeedCache):
- `AddPost` — ZADD + ZREMRANGEBYRANK + EXPIRE (pipeline)
- `RemovePost` — ZREM
- `GetFeed` — ZREVRANGEBYSCORE or ZREVRANGE
- `GetScore` — ZSCORE
- `WarmCache` — bulk ZADD (pipeline)

---

## Implementation Progress

### Phase 1: Redis Foundation
- [ ] Add `github.com/redis/go-redis/v9` to `go.mod` (currently not required)
- [ ] Set up Redis connection (single shared client)
- [ ] Add `REDIS_URL` config to `.env`
- [ ] Verify connection on app startup (fail fast)

### Phase 2: Feed Cache
- [ ] Create FeedCache interface (`internal/cache/feed.go`)
- [ ] Implement AddPost (pipeline: ZADD + ZREMRANGEBYRANK + EXPIRE)
- [ ] Implement RemovePost
- [ ] Implement GetFeed (with cursor support)
- [ ] Implement GetScore
- [ ] Implement WarmCache (bulk insert)

### Phase 3: Message Queue
- [ ] Define event types (`internal/queue/events.go`)
- [ ] Implement Publisher (`internal/queue/publisher.go`)
- [ ] Implement Consumer with retry logic (`internal/queue/consumer.go`)
- [ ] Implement Worker Manager (`internal/worker/manager.go`)

### Phase 4: Feed Integration
- [ ] Post creation → publish `post.created` event
- [ ] Feed fanout worker → add to followers' caches
- [ ] Post deletion → publish `post.deleted` event
- [ ] Cleanup worker → remove from followers' caches
- [ ] Follow → publish `user.followed` event
- [ ] Unfollow → publish `user.unfollowed` event

### Phase 5: Feed Service
- [ ] Implement GET /feed endpoint
- [ ] Cold start (empty cache → warm from DB)
- [ ] Cursor parsing (compound format: `id:timestamp`)
- [ ] Cache fallback to DB when cursor not found

### Phase 6: Notifications (Future)
- [ ] Notification cache structure (TBD)
- [ ] Like notification worker
- [ ] Comment notification worker
- [ ] Reply notification worker
- [ ] Follow notification worker

---

## Out of Scope (For Now)

- Graceful shutdown handling
- Metrics/observability
- Dead-letter queue alerting
- Rate limiting on fan-out

---

## Prerequisites (before wiring feed end-to-end)

- **DB schema:** current `migrations/` does not include `posts` / `post_details` yet.
- **Route naming:** confirm whether API paths are `/post` or `/posts` so router/handlers match.

---

## Best Practices

### Redis Connection
- **One client, shared everywhere** — Don't create new connections per request
- **Ping on startup** — Fail fast if Redis is unreachable
- **Use context with timeout** — Prevent hanging on Redis issues

### Pipelines
- **Batch related commands** — AddPost should pipeline ZADD + ZREMRANGEBYRANK + EXPIRE together
- **Don't over-pipeline** — Keep pipelines under ~100 commands for readability

### Error Handling
- **Cache failures are NOT fatal** — Feed should still work if Redis is down (DB fallback)
- **Log, don't panic** — Redis errors should log and degrade gracefully
- **Wrap errors with context** — `fmt.Errorf("add post to feed: %w", err)`

### Message Queue
- **Idempotent handlers** — Same message processed twice should have same result
- **Small payloads** — Store IDs in events, not full objects (hydrate from DB)
- **One responsibility per handler** — Don't mix cache updates with notifications

---

## Do's and Don'ts

### ✅ Do

| Practice | Why |
|----------|-----|
| Use interfaces for cache layer | Enables testing with mocks |
| Use pipelines for multi-command ops | Reduces round trips |
| Include timestamp in cursor | Enables DB fallback when post deleted |
| Fail open on Redis errors | Users shouldn't see errors for cache issues |
| Log consumer errors with message ID | Makes debugging easier |

### ❌ Don't

| Anti-pattern | Why Not |
|--------------|---------|
| Create Redis client per request | Connection overhead kills performance |
| Store full post data in cache | Wastes memory; IDs are enough, hydrate from DB |
| Block request on queue publish failure | Queue is async; log and continue |
| Retry indefinitely | Dead letters exist for a reason; cap at 3 retries |
| Use KEYS command in production | O(N) scan blocks Redis; use SCAN if needed |
| Ignore PEL (Pending Entries List) | Stuck messages = lost work; reclaim them |

