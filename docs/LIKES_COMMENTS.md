# Likes & Comments API

Tài liệu này mô tả **các endpoint liên quan đến Likes và Comments** đã được implement trong backend Go.

Mục tiêu: đủ rõ ràng để team frontend có thể implement theo.

---

## Mục lục
1. [Authentication](#authentication)
2. [Data models (TypeScript types)](#data-models-typescript-types)
3. [Likes](#likes)
   - [POST /posts/{id}/likes](#post-postsidlikes)
   - [DELETE /posts/{id}/likes](#delete-postsidlikes)
   - [GET /posts/{id}/likes](#get-postsidlikes)
4. [Comments](#comments)
   - [POST /posts/{id}/comments](#post-postsidcomments)
   - [PATCH /posts/{id}/comments/{commentId}](#patch-postsidcommentscommentid)
   - [DELETE /posts/{id}/comments/{commentId}](#delete-postsidcommentscommentid)
   - [GET /posts/{id}/comments](#get-postsidcomments)

---

## Authentication

- **Tất cả endpoint trong tài liệu này đều yêu cầu authentication.**
- Mobile nên dùng Authorization header:

```http
Authorization: Bearer <access_token>
```

### Format lỗi (chuẩn chung)
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Thông điệp lỗi dễ hiểu"
  }
}
```

---

## Data models (TypeScript types)

### Comment
```ts
export type Comment = {
  id: number;
  post_id: number;
  content: string;
  parent_comment_id?: number | null; // null = top-level comment
  created_at: string; // ISO string
  author?: UserSummary;
};

export type UserSummary = {
  id: number;
  username: string;
  display_name: string | null;
  avatar_url: string | null;
  is_following: boolean;
};
```

### CommentListResponse
```ts
export type CommentListResponse = {
  comments: Comment[];
  next_cursor?: string; // chỉ có khi has_more = true
  has_more: boolean;
};
```

### LikersListResponse
```ts
export type LikersListResponse = {
  users: UserSummary[];
  next_cursor?: string; // chỉ có khi has_more = true
  has_more: boolean;
};
```

---

## Likes

### POST /posts/{id}/likes

Like một post.

**Auth:** Bắt buộc

#### Request
```http
POST /posts/123/likes
Authorization: Bearer <access_token>
```

Không cần request body.

#### Response (201 Created)
```json
{
  "message": "Post liked successfully"
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `404 NOT_FOUND`: post không tồn tại
- `409 CONFLICT`: đã like post này rồi

#### Side effects
- Insert vào `post_likes`
- `posts.like_count = like_count + 1` (trong cùng transaction)

---

### DELETE /posts/{id}/likes

Bỏ like một post.

**Auth:** Bắt buộc

#### Request
```http
DELETE /posts/123/likes
Authorization: Bearer <access_token>
```

#### Response (200 OK)
```json
{
  "message": "Post unliked successfully"
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `404 NOT_FOUND`: chưa like post này hoặc post không tồn tại

#### Side effects
- Delete from `post_likes`
- `posts.like_count = like_count - 1` (trong cùng transaction)

---

### GET /posts/{id}/likes

Lấy danh sách users đã like một post.

**Auth:** Bắt buộc

#### Request
```http
GET /posts/123/likes?cursor=<cursor>&limit=10
Authorization: Bearer <access_token>
```

Query params:
- `limit` (optional): default `10`, max `50`
- `cursor` (optional): cursor pagination do backend trả về

#### Cursor format
- Format: `<like_id>:<unix_timestamp>`
- Ví dụ: `456:1734439200`
- Frontend nên treat cursor là **opaque**.

#### Response (200 OK)
```json
{
  "users": [
    {
      "id": 501,
      "username": "johndoe",
      "display_name": "John Doe",
      "avatar_url": "https://..."
    }
  ],
  "next_cursor": "456:1734439200",
  "has_more": true
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `404 NOT_FOUND`: post không tồn tại
- `400 BAD_REQUEST`: limit không hợp lệ

---

## Comments

### POST /posts/{id}/comments

Tạo comment trên một post.

**Auth:** Bắt buộc

#### Request
```http
POST /posts/123/comments
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "content": "Great post!",
  "parent_comment_id": null
}
```

Fields:
- `content` (required): nội dung comment, max **2200 ký tự**
- `parent_comment_id` (optional): ID của comment cha nếu đây là reply

> 💡 **Facebook-style reply**: Backend sử dụng cơ chế tương tự Facebook. Nếu bạn reply vào một reply (nested reply), backend sẽ tự động:
> 1. **Flatten**: Comment của bạn sẽ được gắn vào comment gốc (top-level) thay vì reply
> 2. **@mention**: Tự động thêm `@username` vào đầu nội dung để tag người bạn đang reply
>
> Ví dụ: Reply vào comment của "alice" (đã là reply) → content sẽ thành `@alice <nội dung của bạn>`

#### Response (201 Created)
```json
{
  "id": 789,
  "post_id": 123,
  "content": "Great post!",
  "parent_comment_id": null,
  "created_at": "2025-12-18T10:00:00Z",
  "author": {
    "id": 1,
    "username": "alice",
    "display_name": "Alice",
    "avatar_url": "https://..."
  }
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `404 NOT_FOUND`: post không tồn tại hoặc parent comment không tồn tại
- `400 BAD_REQUEST`:
  - "Comment content is required"
  - "Comment content too long"

#### Side effects
- Insert vào `post_comments`
- `posts.comment_count = comment_count + 1` (trong cùng transaction)

---

### PATCH /posts/{id}/comments/{commentId}

Sửa nội dung comment. Chỉ chủ comment mới được sửa.

**Auth:** Bắt buộc

#### Request
```http
PATCH /posts/123/comments/789
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "content": "Updated comment content!"
}
```

Fields:
- `content` (required): nội dung mới, max **2200 ký tự**

#### Response (200 OK)
```json
{
  "id": 789,
  "post_id": 123,
  "content": "Updated comment content!",
  "parent_comment_id": null,
  "created_at": "2025-12-18T10:00:00Z",
  "author": {
    "id": 1,
    "username": "alice",
    "display_name": "Alice",
    "avatar_url": "https://..."
  }
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `403 FORBIDDEN`: không phải chủ comment
- `404 NOT_FOUND`: comment không tồn tại
- `400 BAD_REQUEST`:
  - "Comment content is required"
  - "Comment content too long"

---

### DELETE /posts/{id}/comments/{commentId}

Xóa comment. Chỉ chủ comment mới được xóa.

**Auth:** Bắt buộc

#### Request
```http
DELETE /posts/123/comments/789
Authorization: Bearer <access_token>
```

#### Response (200 OK)
```json
{
  "message": "Comment deleted successfully"
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `403 FORBIDDEN`: không phải chủ comment
- `404 NOT_FOUND`: comment không tồn tại

#### Side effects
- Delete from `post_comments`
- `posts.comment_count = comment_count - 1` (trong cùng transaction)

---

### GET /posts/{id}/comments

Lấy danh sách comments của một post.

**Auth:** Bắt buộc

#### Request
```http
GET /posts/123/comments?cursor=<cursor>&limit=10
Authorization: Bearer <access_token>
```

Query params:
- `limit` (optional): default `10`, max `50`
- `cursor` (optional): cursor pagination do backend trả về

#### Cursor format
- Format: `<comment_id>:<unix_timestamp>`
- Ví dụ: `789:1734439200`
- Frontend nên treat cursor là **opaque**.

#### Response (200 OK)
```json
{
  "comments": [
    {
      "id": 789,
      "post_id": 123,
      "content": "Great post!",
      "parent_comment_id": null,
      "created_at": "2025-12-18T10:00:00Z",
      "author": {
        "id": 501,
        "username": "johndoe",
        "display_name": "John Doe",
        "avatar_url": "https://..."
      }
    }
  ],
  "next_cursor": "789:1734439200",
  "has_more": true
}
```

Field notes:
- Comments được sắp xếp theo `created_at DESC` (mới nhất trước)
- `parent_comment_id` sẽ có giá trị nếu đây là reply của comment khác

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `404 NOT_FOUND`: post không tồn tại
- `400 BAD_REQUEST`: limit không hợp lệ

---

## Ghi chú quan trọng

1. **Atomic transactions**: Tất cả operations like/unlike và comment create/delete đều dùng database transaction để đảm bảo counter (`like_count`, `comment_count`) luôn consistent với số lượng thực tế trong table.

2. **Facebook-style 1-level reply**: Backend chỉ lưu trữ 1 level reply trong DB. Khi user reply vào một reply, backend sẽ:
   - **Flatten**: Tự động chuyển `parent_comment_id` sang comment gốc (top-level)
   - **@mention**: Prepend `@username` vào content để tag người được reply
   
   Điều này giúp UI đơn giản hơn (chỉ cần hiển thị 2 level) trong khi vẫn giữ context ai đang reply ai.

3. **Pagination**: Tất cả list endpoints đều dùng cursor-based pagination với format `id:timestamp`. Đây là format thống nhất với các endpoint khác trong hệ thống.
