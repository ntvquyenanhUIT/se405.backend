# Tài Liệu Follow System API

## Mục Lục
1. [Tổng Quan](#tổng-quan)
2. [API Endpoints](#api-endpoints)
3. [Pagination Guide](#pagination-guide)
4. [TypeScript Types](#typescript-types)
5. [Frontend Implementation Guide](#frontend-implementation-guide)
6. [FAQ](#faq)

---

## Tổng Quan

### Kiến Trúc Follow System
Hệ thống follow sử dụng kiến trúc đơn giản nhưng hiệu quả:

- **Database**: Bảng `follows` với composite primary key `(follower_id, followee_id)`
- **Counters**: `follower_count` và `following_count` được update trong transaction
- **Pagination**: Cursor-based pagination sử dụng `created_at` timestamp
- **Batch Queries**: Sử dụng PostgreSQL `ANY($1)` để check follow status cho nhiều users cùng lúc (tránh N+1 queries)

### Follow Relationship
```
User A follows User B
  ↓
- A.following_count + 1
- B.follower_count + 1
- Record: (follower_id=A, followee_id=B) in follows table
```

### Optional Authentication
Các endpoints liên quan đến follow system hỗ trợ **optional authentication**:
- **Có token**: Trả về `is_following` status chính xác
- **Không có token**: Vẫn trả về data nhưng `is_following` luôn = `false`

**LƯU Ý FRONTEND:**
- Nếu muốn hiển thị button "Follow/Following" → **PHẢI** gửi token
- Nếu chỉ xem danh sách → Có thể không gửi token

---

## API Endpoints

### 1. Follow User

**Chức năng**: Bắt đầu follow một user

#### Request
```http
POST /users/:id/follow
Authorization: Bearer <access_token> (REQUIRED)
```

**Path Parameters:**
- `id` (integer, required): ID của user cần follow

**LƯU Ý FRONTEND:**
- Endpoint này **BẮT BUỘC** phải có authentication
- Không thể follow chính mình (server sẽ trả lỗi)
- Phải check `id !== currentUserId` ở client trước khi gọi API

#### Response Success (200 OK)
```json
{
  "message": "Successfully followed user"
}
```

#### Frontend Implementation

**Workflow sau khi gọi API:**

1. **Cập nhật UI ngay lập tức (Optimistic Update)**:
   - Đổi button state từ "Follow" → "Following" (hoặc ngược lại)
   - Increment/decrement follower count trên profile (nếu đang xem profile của user đó)
   - Disable button để prevent double-click
   - Add loading spinner (optional)

2. **Handle errors và rollback**:
   - Nếu request fail → revert button state về trạng thái ban đầu
   - Show error toast với message rõ ràng
   - Re-enable button

3. **Update cache/state management**:
   - Invalidate user profile cache
   - Invalidate follower/following lists cache
   - Update `is_following` field trong tất cả places hiển thị user đó

4. **Background sync**:
   - Nếu app offline → queue action để sync sau
   - Khi reconnect → retry failed actions

#### Error Responses

**409 Conflict - Already Following**
```json
{
  "error": {
    "code": "ALREADY_FOLLOWING",
    "message": "Already following this user"
  }
}
```

**Frontend cần handle:**
- **KHÔNG** show error message (vì UI đã show "Following")
- Chỉ cần ensure button state đúng
- Có thể log warning để debug

**404 Not Found - User Not Exists**
```json
{
  "error": {
    "code": "USER_NOT_FOUND",
    "message": "User not found"
  }
}
```

**Frontend cần handle:**
- Show error toast: "User not found"
- Có thể navigate back hoặc refresh profile

**400 Bad Request - Cannot Follow Self**
```json
{
  "error": {
    "code": "INVALID_ACTION",
    "message": "Cannot follow yourself"
  }
}
```

**Frontend cần handle:**
- Đây là bug ở client (không nên xảy ra)
- Hide follow button khi xem own profile

**401 Unauthorized**
```json
{
  "error": {
    "code": "TOKEN_EXPIRED",
    "message": "Access token has expired"
  }
}
```

**Frontend cần handle:**
- Clear token và navigate to login
- Show message: "Please log in again"

**500+ Server Error**

**Frontend cần handle:**
- Show retry button
- Rollback optimistic update
- Keep previous button state

---

### 2. Unfollow User

**Chức năng**: Ngừng follow một user

#### Request
```http
DELETE /users/:id/follow
Authorization: Bearer <access_token> (REQUIRED)
```

**Path Parameters:**
- `id` (integer, required): ID của user cần unfollow

#### Response Success (200 OK)
```json
{
  "message": "Successfully unfollowed user"
}
```

#### Frontend Implementation

**Workflow sau khi gọi API:**

1. **Update UI ngay lập tức (Optimistic Update)**:
   - Đổi button state từ "Following" → "Follow"
   - Decrement follower count
   - Disable button để prevent double-click

2. **Handle errors và rollback**:
   - Nếu fail → revert button về "Following"
   - Show error toast
   - Re-enable button

3. **Không cần confirmation dialog** (theo Instagram pattern):
   - User có thể re-follow ngay nếu unfollow nhầm
   - UX mượt hơn

4. **Update cache**:
   - Invalidate profile cache
   - Invalidate follower list (user sẽ biến mất khỏi follower list của người kia)

#### Error Responses

**404 Not Found - Not Following**
```json
{
  "error": {
    "code": "RELATIONSHIP_NOT_FOUND",
    "message": "Not following this user"
  }
}
```

**Frontend cần handle:**
- **KHÔNG** show error
- Chỉ ensure button state = "Follow"

**404 Not Found - User Not Exists**
```json
{
  "error": {
    "code": "USER_NOT_FOUND",
    "message": "User not found"
  }
}
```

**Frontend cần handle:**
- Show error: "User not found"
- Navigate back

**401 Unauthorized**

**Frontend cần handle:**
- Navigate to login

**500+ Server Error**

**Frontend cần handle:**
- Show retry button
- Rollback optimistic update

---

### 3. Get Followers List

**Chức năng**: Lấy danh sách những người đang follow một user

#### Request
```http
GET /users/:id/followers?cursor={cursor}&limit={limit}
Authorization: Bearer <access_token> (OPTIONAL)
```

**Path Parameters:**
- `id` (integer, required): ID của user

**Query Parameters:**
- `cursor` (string, optional): Pagination cursor (RFC3339 timestamp)
  - Không truyền hoặc `null` → lấy page đầu tiên
  - Pass cursor từ response trước để load more
- `limit` (integer, optional): Số lượng items per page
  - Default: 20
  - Min: 1, Max: 100

**Authentication:**
- **Optional**: Có thể gọi không cần token
- **Có token**: `is_following` chính xác cho mỗi follower
- **Không token**: `is_following` luôn = `false`

#### Response Success (200 OK)
```json
{
  "users": [
    {
      "id": 123,
      "username": "alice",
      "display_name": "Alice Johnson",
      "avatar_url": "https://r2.example.com/avatars/123.jpg",
      "is_following": true
    },
    {
      "id": 456,
      "username": "bob_smith",
      "display_name": "Bob Smith",
      "avatar_url": "https://r2.example.com/default-avatar.jpg",
      "is_following": false
    }
  ],
  "cursor": "2024-12-01T10:30:00Z",
  "has_more": true
}
```

**Note**: All fields are always present (`display_name` and `avatar_url` are NOT NULL in DB)

**Response Fields:**
- `users`: Array of UserSummary objects (có thể empty)
- `cursor`: Next cursor để load more (null nếu hết data)
- `has_more`: `true` nếu còn data, `false` nếu hết

#### Frontend Implementation

**Workflow khi render list:**

1. **Render danh sách followers**:
   - Loop qua `users` array
   - Hiển thị avatar (always present), display_name (always present), @username
   - Show follow button với `is_following` state

2. **Implement infinite scroll**:
   - Detect khi user scroll đến cuối list (bottom threshold ~100px)
   - Load more nếu còn data (`has_more === true`)
   - Pass `cursor` từ response trước vào request tiếp theo
   - Append new users vào existing list (không replace!)
   - Show loading spinner ở cuối list

3. **Handle `has_more`**:
   - `has_more === true`: Còn data → continue loading
   - `has_more === false`: Hết data → hide loading spinner, có thể show "No more users" text

4. **Update sau khi follow/unfollow**:
   - Optimistic update: Đổi `is_following` ngay trong list
   - Không cần refetch cả list

5. **Empty state**:
   - `users.length === 0` VÀ `has_more === false`: Show "No followers yet"

#### Error Responses

**404 Not Found**
```json
{
  "error": {
    "code": "USER_NOT_FOUND",
    "message": "User not found"
  }
}
```

**Frontend cần handle:**
- Show error screen: "User not found"
- Button: "Go Back"

**Empty List**

**Frontend cần handle:**
- Show empty state: "No followers yet"
- Có thể suggest: "Be the first to follow!"

**Network Error**

**Frontend cần handle:**
- Show error screen với retry button
- Keep previous data nếu có (graceful degradation)

---

### 4. Get Following List

**Chức năng**: Lấy danh sách những người mà user đang follow

#### Request
```http
GET /users/:id/following?cursor={cursor}&limit={limit}
Authorization: Bearer <access_token> (OPTIONAL)
```

**Path Parameters:**
- `id` (integer, required): ID của user

**Query Parameters:**
- `cursor` (string, optional): Pagination cursor (RFC3339 timestamp)
- `limit` (integer, optional): Số lượng items per page (default: 20, max: 100)

**Authentication:** Same as Followers list

#### Response Success (200 OK)
```json
{
  "users": [
    {
      "id": 789,
      "username": "charlie",
      "display_name": "Charlie Brown",
      "avatar_url": "https://r2.example.com/avatars/789.jpg",
      "is_following": true
    }
  ],
  "cursor": "2024-12-01T09:15:00Z",
  "has_more": false
}
```

**LƯU Ý:** 
- Trong following list, `is_following` của current user sẽ luôn = `true` cho tất cả users (vì đó là list những người mình đang follow)
- Nhưng nếu có viewer khác xem list này, `is_following` sẽ khác nhau

#### Frontend Implementation

**Workflow khi render list:**

1. **Render danh sách following**:
   - Loop qua `users` array
   - Show avatar (always present), display_name (always present), @username
   - Show "Following" button (vì `is_following` luôn là `true` trong list này)

2. **Implement infinite scroll**: Giống hệt như Followers list
   - Detect scroll to bottom
   - Load more với cursor
   - Append to existing list

3. **Handle unfollow action**:
   - Khi user unfollow someone TRONG list này
   - Optimistic update: Remove user khỏi list ngay lập tức
   - Nếu API fail → add user lại vào list

4. **Empty state**:
   - Show: "Not following anyone yet"
   - Suggest: "Find people to follow"

#### Error Responses

Same as Followers list endpoint.

---

## Pagination Guide

### Cursor-Based Pagination

**Concept:**
- Dùng `created_at` timestamp làm cursor
- Stable: Data mới không ảnh hưởng previous pages
- Fast: Database dùng index trên `created_at`

**Workflow:**

1. **Initial Load** (First Page):
   - Gọi API không có cursor: `GET /users/:id/followers?limit=20`
   - Nhận response: `{ users: [...], cursor: "2024-12-01T10:00:00Z", has_more: true }`
   - Render 20 items đầu tiên

2. **Load More** (Subsequent Pages):
   - User scroll đến cuối list
   - Check `has_more === true`
   - Gọi API với cursor: `GET /users/:id/followers?cursor=2024-12-01T10:00:00Z&limit=20`
   - Append new users vào existing list

3. **End of List**:
   - Khi `has_more === false`: Đã hết data
   - Hide loading spinner
   - Show "End of list" message (optional)

**Important Notes:**

- **Không sử dụng offset pagination** (`?page=2&limit=20`) vì có vấn đề với data consistency
- **Cursor là timestamp**: RFC3339 format, ví dụ: `2024-12-01T10:30:00Z`
- **Cursor có thể null**: Khi hết data, backend trả `cursor: null`
- **Limit mặc định**: 20 items (balance giữa performance và UX)

### Implementation Checklist

**Frontend phải implement:**
- [ ] Initial load không có cursor
- [ ] Detect scroll to bottom (threshold ~100px)
- [ ] Check `has_more` trước khi load more
- [ ] Prevent duplicate requests (flag `isLoadingMore`)
- [ ] Append data (không replace)
- [ ] Handle loading states (initial, loading more)
- [ ] Handle empty state (`users.length === 0`)
- [ ] Handle error state (show retry button)
- [ ] Cache data (optional, 30-60 seconds)

### Edge Cases

**Case 1: New follower appears**
- User A đang scroll followers list
- User B follow trong lúc đó
- User A load next page → Không thấy User B (vì cursor filtering)
- Solution: Pull-to-refresh để get latest data

**Case 2: Someone unfollows**
- User đang scroll
- Ai đó unfollow → item biến mất
- Cursor vẫn work, không bị lỗi

**Case 3: Network timeout**
- Request timeout → Show retry button
- Keep previous data
- Không clear list

---

## TypeScript Types

```typescript
/**
 * User summary (compact version)
 * Dùng cho: Followers/Following lists, Search results
 */
interface UserSummary {
  id: number;
  username: string;
  display_name: string;              // Always present (NOT NULL in DB)
  avatar_url: string;                // Always present (NOT NULL in DB)
  is_following: boolean;             // false nếu không có token
}

/**
 * Follow action response
 * Dùng cho: POST /users/:id/follow, DELETE /users/:id/follow
 */
interface FollowActionResponse {
  message: string;                   // "Successfully followed user" hoặc "Successfully unfollowed user"
}

/**
 * Follow list response (Followers hoặc Following)
 * Dùng cho: GET /users/:id/followers, GET /users/:id/following
 */
interface FollowListResponse {
  users: UserSummary[];              // Array of users (có thể empty)
  cursor: string | null;             // Next cursor (null nếu hết data)
  has_more: boolean;                 // true = còn data, false = hết
}

/**
 * API Methods
 */
interface FollowAPI {
  // Follow a user
  follow(userId: number): Promise<FollowActionResponse>;
  
  // Unfollow a user
  unfollow(userId: number): Promise<FollowActionResponse>;
  
  // Get followers list with pagination
  getFollowers(
    userId: number,
    cursor?: string | null,
    limit?: number
  ): Promise<FollowListResponse>;
  
  // Get following list with pagination
  getFollowing(
    userId: number,
    cursor?: string | null,
    limit?: number
  ): Promise<FollowListResponse>;
}
```

**Type Usage Examples:**

```typescript
// Follow/Unfollow actions
const result: FollowActionResponse = await api.follow(userId);

// Get followers with pagination
const page1: FollowListResponse = await api.getFollowers(userId, null, 20);
const page2: FollowListResponse = await api.getFollowers(userId, page1.cursor, 20);

// User card component
interface UserCardProps {
  user: UserSummary;
  onFollowToggle: (userId: number, isFollowing: boolean) => void;
}
```

---

## Frontend Implementation Guide

### Pattern 1: Follow Button Component

**Component cần:**
- **Props**: `userId`, `initialFollowing` state
- **State**: `isFollowing`, `isLoading`
- **Logic**: 
  - Toggle follow/unfollow on click
  - Optimistic update
  - Error handling + rollback
  - Disable button khi loading

**Button states:**
- `isFollowing === false`: Show "Follow" button (primary color)
- `isFollowing === true`: Show "Following" button (secondary color)
- `isLoading === true`: Show spinner, disable button

**Integration:**
- Dùng trong profile screen, user cards, search results, followers/following lists
- Must be reusable component

### Pattern 2: Followers/Following Lists with Infinite Scroll

**List component cần:**
- **Props**: `userId`, `type` ('followers' | 'following')
- **State**: `users` array, `cursor`, `hasMore`, `isLoading`, `isLoadingMore`
- **Features**:
  - Initial load: Fetch first page (limit 20)
  - Infinite scroll: Detect bottom threshold → load more
  - Loading states: Initial spinner, bottom spinner
  - Empty state: "No followers/following yet"
  - Error state: Retry button

**Scroll detection:**
- Detect khi user scroll đến ~100px từ bottom
- Check `hasMore === true` before loading
- Prevent duplicate requests (check `isLoadingMore`)

**Data management:**
- Append new users to existing array (không replace)
- Update cursor sau mỗi load
- Set `hasMore` từ API response

### Pattern 3: User Card in Lists

**Card component cần:**
- **Display**: Avatar (always present), display_name (always present), @username
- **Action**: Follow/Following button (hide nếu own profile)
- **Interaction**: Tap card → navigate to profile
- **Props**: `user` object, `currentUserId`, `onUserPress` callback

**Follow button integration:**
- Pass `user.id` và `user.is_following` to FollowButton
- Update `is_following` optimistically when toggled
- Không cần refetch entire list

### Pattern 4: Profile Tabs with Counters

**Tab structure:**
- Tab 1: Posts (count: `post_count`)
- Tab 2: Followers (count: `follower_count`, tappable)
- Tab 3: Following (count: `following_count`, tappable)

**Counter updates:**
- Khi follow/unfollow → increment/decrement local counter ngay
- Không cần refetch profile
- Eventual consistency: Profile sẽ được refetch khi navigate back

**Navigation:**
- Tap counter → navigate to respective list screen
- Highlight active tab
- Lazy load tab content

## Summary

**Key Points:**
- `POST /users/:id/follow`: Follow user (requires auth, optimistic updates recommended)
- `DELETE /users/:id/follow`: Unfollow user (requires auth, no confirmation needed)
- `GET /users/:id/followers`: List followers with cursor pagination (optional auth)
- `GET /users/:id/following`: List following with cursor pagination (optional auth)
- Cursor pagination: Stable, fast, consistent
- Optimistic updates: Better UX, must implement rollback
- Error handling: Different UI cho different error types
- Cache strategy: 30-60 seconds for lists, invalidate on follow/unfollow
