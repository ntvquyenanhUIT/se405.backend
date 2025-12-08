# Tài Liệu User Profile & Discovery API

## Mục Lục
1. [Tổng Quan](#tổng-quan)
2. [API Endpoints](#api-endpoints)
3. [TypeScript Types](#typescript-types)
4. [Frontend Implementation Guide](#frontend-implementation-guide)
5. [FAQ](#faq)

---

## Tổng Quan

### Kiến Trúc
System này cung cấp 2 chức năng chính:

- **View User Profile**: Xem thông tin chi tiết của bất kỳ user nào (public profiles)
- **Search Users**: Tìm kiếm users theo username với prefix matching
- **Optional Auth**: Hỗ trợ cả authenticated và anonymous access
- **Follow Status**: Tự động check follow relationship nếu có token

**LƯU Ý QUAN TRỌNG**: 
- **`GET /me`**: Dùng để verify token và get basic current user info (không có `is_following`)
- **`GET /users/:id`**: Dùng để view PROFILE của bất kỳ user nào (kể cả chính mình)
  - Khi view own profile: `GET /users/{myId}` → `is_following` = false (không thể follow chính mình)
  - Khi view others: `GET /users/{otherId}` → `is_following` = true/false
  
**Use case pattern:**
- App startup / Token verification → `GET /me`
- Profile screen (own or others) → `GET /users/:id`
- Posts tab in profile → `GET /users/:id/posts` (future implementation)

---

## API Endpoints

### 1. Get User Profile

**Chức năng**: Xem thông tin chi tiết của một user (public profile)

#### Request
```http
GET /users/:id
Authorization: Bearer <access_token> (OPTIONAL)
```

**Path Parameters:**
- `id` (integer, required): ID của user cần xem

**Authentication:**
- **Optional**: Có thể gọi mà không cần token
- **Có token**: Trả về `is_following` status chính xác
- **Không token**: `is_following` luôn = `false`

#### Response Success (200 OK)
```json
{
  "id": 123,
  "username": "johndoe",
  "display_name": "John Doe",
  "bio": "Software Engineer | Coffee lover",
  "avatar_url": "https://r2.example.com/avatars/123.jpg",
  "is_new_user": false,
  "follower_count": 1250,
  "following_count": 340,
  "post_count": 89,
  "is_following": true,
  "created_at": "2024-11-15T08:30:00Z",
  "updated_at": "2024-12-01T14:22:00Z"
}
```

**Field Guarantees:**
- `display_name`: **Always present** (required during registration, NOT NULL in DB)
- `avatar_url`: **Always present** (either uploaded or default avatar from config, NOT NULL in DB)
- `bio`: **Can be null** → Ẩn bio section if null

**Field `is_following`:**
- `true`: Current user đang follow user này
- `false`: Chưa follow HOẶC không có token
- `null`: Không bao giờ xảy ra (luôn có giá trị boolean)

**LƯU Ý:**
- Nếu view chính mình: `is_following` = `false` (không thể follow chính mình)
- Counter (`follower_count`, `following_count`, `post_count`) luôn >= 0

#### Frontend Implementation

**Workflow sau khi nhận response:**

1. **Render profile UI** với đầy đủ thông tin:
   - Avatar (always present - direct render từ `avatar_url`)
   - Display name (always present - direct render từ `display_name`)
   - Username (hiển thị dạng @username)
   - Bio (ẩn section nếu null)
   - Stats: Posts, Followers, Following counts (có thể tap để navigate)

2. **Check xem có phải profile của mình không**:
   - So sánh `profile.id` với `currentUserId`
   - Nếu là profile của mình: Show "Edit Profile" button, ẩn "Follow" button
   - Nếu không phải: Show "Follow/Following" button

3. **Handle `is_new_user` flag**:
   - Nếu xem chính mình VÀ `is_new_user === true`: Navigate to onboarding flow
   - User cần complete profile, find friends, etc.

4. **Handle null fields gracefully**:
   - `bio` null → ẩn bio section hoàn toàn
   - `display_name` và `avatar_url` luôn có giá trị (guaranteed by backend)

5. **Format timestamps** cho user-friendly:
   - `created_at` → "Joined November 2025"
   - `updated_at` → "Active 2 days ago" (optional)

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
- Show error screen: "User not found" / "This user doesn't exist"
- Action button: "Go Back"

**400 Bad Request**
```json
{
  "error": {
    "code": "INVALID_USER_ID",
    "message": "Invalid user ID"
  }
}
```

**Frontend cần handle:**
- Show error screen: "Invalid user"
- Action button: "Go Back"

**500+ Server Error**

**Frontend cần handle:**
- Show error screen: "Something went wrong" / "Unable to load profile"
- Action button: "Retry"

---

### 2. Search Users

**Chức năng**: Tìm kiếm users theo username (prefix matching, case-insensitive)

#### Request
```http
GET /users/search?q={query}&limit={limit}
Authorization: Bearer <access_token> (OPTIONAL)
```

**Query Parameters:**
- `q` (string, required): Search query (username prefix)
  - Min length: 2 characters
  - Case-insensitive
  - Prefix matching (search "joh" → finds "johndoe", "john_smith")
- `limit` (integer, optional): Số lượng results tối đa
  - Default: 20
  - Min: 1, Max: 100

**Authentication:**
- **Optional**: Có thể gọi không cần token
- **Có token**: `is_following` chính xác cho mỗi user
- **Không token**: `is_following` luôn = `false`

#### Response Success (200 OK)
```json
{
  "users": [
    {
      "id": 123,
      "username": "johndoe",
      "display_name": "John Doe",
      "avatar_url": "https://r2.example.com/avatars/123.jpg",
      "is_following": false
    },
    {
      "id": 456,
      "username": "john_smith",
      "display_name": "John Smith",
      "avatar_url": "https://r2.example.com/default-avatar.jpg",
      "is_following": true
    }
  ]
}
```

**Note**: All users have `display_name` and `avatar_url` (backend guarantees NOT NULL)

**Results Behavior:**
- **Sort order**: Theo `follower_count DESC` (popular users first)
- **Empty query**: Trả về popular users (top users by follower count)
- **No results**: `users` = empty array `[]`

#### Frontend Implementation

**Workflow:**

**Option 1: Search as You Type (Debounced)**
- Setup search input với debounce 300ms để tránh gọi API quá nhiều
- Chỉ search khi query có ít nhất 2 ký tự
- Clear results khi query < 2 ký tự
- Show loading spinner trong khi search
- Show empty state nếu không có results

**Option 2: Search with Button**
- User gõ query → nhấn Search button → gọi API
- Validate query >= 2 ký tự trước khi gọi API
- Disable button trong khi loading
- Không cần debounce vì user control khi nào search

**Option 3: Search with Cache**
- Cache search results trong 30 giây - 5 phút
- Reuse cached results nếu search lại cùng query
- Invalidate cache khi follow/unfollow (vì `is_following` thay đổi)
- Sử dụng query library (React Query, SWR, RTK Query, etc.)

#### Input Validation

**Frontend cần handle:**

- **Validate query trước khi gọi API**:
  - Query không được empty hoặc chỉ có spaces
  - Query phải có ít nhất 2 ký tự
  - Show error message: "Please enter at least 2 characters"

- **Validate limit**:
  - Nếu limit < 1 hoặc > 100: dùng default 20
  - Console.warn để debug

- **Encode query**:
  - Sử dụng URL encoding cho query parameter
  - Handle special characters (@, #, spaces, etc.)

#### Error Responses

**400 Bad Request**
```json
{
  "error": {
    "code": "INVALID_QUERY",
    "message": "Query must be at least 2 characters"
  }
}
```

**Frontend cần handle:**
- Show inline error message dưới search input
- Không navigate away, user có thể fix query

**500+ Server Error**

**Frontend cần handle:**
- Show error toast: "Search failed. Please try again."
- Retry button hoặc auto-retry sau 3 giây

---

## TypeScript Types

```typescript
/**
 * User profile với đầy đủ thông tin
 * Dùng cho: GET /users/:id, GET /me responses
 */
interface User {
  id: number;
  username: string;
  display_name: string;              // Always present (NOT NULL in DB)
  bio: string | null;                // Ẩn section nếu null
  avatar_url: string;                // Always present (NOT NULL in DB)
  is_new_user: boolean;              // true = cần onboarding
  follower_count: number;            // >= 0
  following_count: number;           // >= 0
  post_count: number;                // >= 0
  is_following: boolean;             // false nếu không có token hoặc xem chính mình
  created_at: string;                // ISO 8601 timestamp
  updated_at: string;                // ISO 8601 timestamp
}

/**
 * User summary (compact version)
 * Dùng cho: Search results, followers/following lists
 */
interface UserSummary {
  id: number;
  username: string;
  display_name: string;              // Always present (NOT NULL in DB)
  avatar_url: string;                // Always present (NOT NULL in DB)
  is_following: boolean;
}

/**
 * Profile response (alias của User)
 */
type ProfileResponse = User;

/**
 * Search response
 */
interface SearchResponse {
  users: UserSummary[];              // Empty array nếu không có results
}
```

**Type Usage Examples:**

```typescript
// Profile screen
const profile: User = await api.getProfile(userId);

// Search results
const searchResults: SearchResponse = await api.searchUsers(query);

// User card component props
interface UserCardProps {
  user: UserSummary;
  onPress: (userId: number) => void;
}
```

---

## Frontend Implementation Guide

### Pattern 1: Profile Screen

**Layout Structure:**
- Header: Avatar, Display Name, Username, Bio
- Stats Row: Posts count (tappable), Followers count (tappable), Following count (tappable)
- Action Button: "Edit Profile" (own profile) hoặc "Follow/Following" (other profiles)
- Content Tabs: Posts grid (future: `GET /users/:id/posts`), Followers list, Following list

**Data Fetching:**
- Profile data: `GET /users/:id` (works for own profile và others)
- Posts: `GET /users/:id/posts` (future implementation, same user ID)
- Followers: `GET /users/:id/followers`
- Following: `GET /users/:id/following`
- **Không** cần special case cho own profile - tất cả dùng cùng user ID

**Navigation:**
- Tap Followers count → Navigate to Followers list screen
- Tap Following count → Navigate to Following list screen
- Tap user trong list → Navigate to that user's profile

**State Management:**
- Check `isOwnProfile` để show/hide đúng buttons
- Handle loading, error states
- Refetch profile sau khi follow/unfollow

### Pattern 2: Search with Recent Searches

**Features cần implement:**
- Search input với debounce
- Recent searches list (lưu local, max 10 items)
- Show recent searches khi query < 2 ký tự
- Clear recent searches button
- Tap user → add to recent + navigate to profile

**Storage:**
- Lưu recent searches vào local storage/AsyncStorage
- Format: array of UserSummary objects
- Update list khi user tap vào result

### Pattern 3: Reusable User Card Component

**Props:**
- `user`: UserSummary hoặc ProfileResponse
- `currentUserId`: để check own profile
- `showFollowButton`: boolean flag
- `onPress`: callback khi tap vào card

**Display Logic:**
- Show avatar (always present, direct render)
- Show display name (always present, direct render)
- Show @username
- Show bio (nếu có trong user object và không null)
- Show stats (nếu có follower_count/following_count)
- Show follow button (nếu không phải own profile)

### Pattern 4: Profile with Tabs

**Tab Structure:**
- Tab 1: Posts grid (TODO: chưa có posts)
- Tab 2: Followers list với infinite scroll
- Tab 3: Following list với infinite scroll

**State Management:**
- Track active tab state
- Lazy load tab content (chỉ load khi tab active)
- Refetch data khi switch tabs


## Summary

**Key Points:**
- `GET /users/:id`: View any user's profile (optional auth)
- `GET /users/search`: Search users by username prefix (optional auth)
- `GET /me`: Get current user info (REQUIRED auth) - xem AUTHENTICATION_FLOW.md
- Null handling: display_name, bio, avatar_url có thể null
- Search: Case-insensitive, prefix matching, sort by popularity
- `is_following`: Requires token, false nếu xem chính mình hoặc không có token
