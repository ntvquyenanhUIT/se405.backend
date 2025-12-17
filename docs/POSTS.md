# Posts API (Tạo / Xem / Xóa) + Upload ảnh post (R2 Presign)

Tài liệu này mô tả **toàn bộ các endpoint liên quan đến Post đã được implement trong backend Go** hiện tại, bao gồm flow **upload ảnh trực tiếp lên Cloudflare R2** bằng **presigned PUT URL**.

Mục tiêu: đủ rõ ràng để team frontend có thể implement theo, hoặc đưa vào AI Agent để generate code.

---

## Mục lục
1. [Authentication](#authentication)
2. [Data models (TypeScript types)](#data-models-typescript-types)
3. [Upload media trực tiếp lên R2](#upload-media-tr%E1%BB%B1c-ti%E1%BA%BFp-l%C3%AAn-r2)
   - [POST /media/posts/presign](#post-mediapostspresign)
   - [POST /media/posts/presign/batch](#post-mediapostspresignbatch)
   - [Bước upload (PUT bytes lên R2)](#b%C6%B0%E1%BB%9Bc-upload-put-bytes-l%C3%AAn-r2)
4. [Feed](#feed)
  - [GET /feed](#get-feed)
5. [Posts](#posts)
  - [POST /posts](#post-posts)
  - [GET /posts/{id}](#get-postsid)
  - [DELETE /posts/{id}](#delete-postsid)
  - [GET /users/{id}/posts](#get-usersidposts)
6. [Ghi chú quan trọng / giới hạn hiện tại](#ghi-ch%C3%BA-quan-tr%E1%BB%8Dng--gi%E1%BB%9Bi-h%E1%BA%A1n-hi%E1%BB%87n-t%E1%BA%A1i)

---

## Authentication

- Endpoint **protected** yêu cầu access token hợp lệ.
- Mobile nên dùng Authorization header:

```http
Authorization: Bearer <access_token>
```

Một số endpoint hỗ trợ **optional auth**: gọi không có token vẫn được, nhưng nếu có token thì backend có thể trả về thêm field cá nhân hóa.

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

### Post
Được trả về bởi `POST /posts` và `GET /posts/{id}`.

```ts
export type Post = {
  id: number;
  user_id: number;
  caption: string | null;
  like_count: number;
  comment_count: number;
  created_at: string; // ISO string
  updated_at: string; // ISO string

  media?: PostMedia[];
  author?: UserSummary;

  // `is_liked` sẽ là true nếu user hiện tại đã like post này
  is_liked?: boolean;
};

export type PostMedia = {
  id: number;
  media_url: string;
  media_type: "image" | "video"; // hiện backend đang default "image"
  position: number; // 0..n-1
};

export type UserSummary = {
  id: number;
  username: string;
  display_name: string | null;
  avatar_url: string | null;

  // NOTE: với post endpoints, backend hiện chưa compute field này (mặc định false)
  is_following: boolean;
};
```

### PostThumbnail (grid trong profile)
Trả về bởi `GET /users/{id}/posts`.

```ts
export type PostThumbnail = {
  id: number;
  thumbnail_url: string; // media đầu tiên
  media_count: number;   // số lượng media trong post
};

export type PostListResponse = {
  posts: PostThumbnail[];
  next_cursor?: string; // chỉ có khi has_more = true
  has_more: boolean;    // có thêm posts không
};
```

---

## Upload media trực tiếp lên R2

Flow upload post image hiện tại:

1) Frontend gọi backend để lấy presigned upload URL.

2) Frontend upload bytes trực tiếp lên R2 bằng **HTTP PUT**.

3) Frontend gọi `POST /posts` và truyền các `public_url` vừa upload thành công vào `media_urls`.

### Constraint / Validation
- Content-Type được hỗ trợ:
  - `image/jpeg`, `image/png`, `image/gif`, `image/webp`
- Giới hạn kích thước: **10MB / ảnh**
- Tối đa **10 media / post**
- Presigned URL hết hạn sau **15 phút** (`expires_in = 900`)

---

### POST /media/posts/presign

Tạo presigned URL để upload **1 ảnh** cho post.

**Auth:** Bắt buộc

#### Request
```http
POST /media/posts/presign
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "content_type": "image/jpeg",
  "file_size": 123456
}
```

- `content_type` (required): phải thuộc list supported.
- `file_size` (optional): nếu gửi thì phải ≤ 10MB.

#### Response (200 OK)
```json
{
  "upload_url": "https://...presigned...",
  "public_url": "https://<public-r2-domain>/posts/<uuid>.jpg",
  "key": "posts/<uuid>.jpg",
  "expires_in": 900
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `400 INVALID_IMAGE_TYPE`: `content_type` không được hỗ trợ
- `400 FILE_TOO_LARGE`: `file_size` vượt quá 10MB

---

### POST /media/posts/presign/batch

Tạo nhiều presigned URL trong **một request**.

**Auth:** Bắt buộc

#### Request
```http
POST /media/posts/presign/batch
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "items": [
    { "content_type": "image/jpeg", "file_size": 123456 },
    { "content_type": "image/png",  "file_size": 234567 }
  ]
}
```

Ràng buộc:
- `items` bắt buộc
- `items.length` phải nằm trong `1..10`

#### Response (200 OK)
```json
{
  "items": [
    {
      "upload_url": "https://...presigned...",
      "public_url": "https://<public>/posts/<uuid>.jpg",
      "key": "posts/<uuid>.jpg",
      "expires_in": 900
    },
    {
      "upload_url": "https://...presigned...",
      "public_url": "https://<public>/posts/<uuid>.png",
      "key": "posts/<uuid>.png",
      "expires_in": 900
    }
  ]
}
```

---

### Bước upload (PUT bytes lên R2)

Sau khi nhận `upload_url`, client upload file bytes:

```http
PUT <upload_url>
Content-Type: image/jpeg

<raw file bytes>
```

Quan trọng:
- **PHẢI dùng PUT** (không dùng POST).
- **PHẢI gửi đúng Content-Type** giống như lúc presign.
- Success thường là `200 OK` hoặc `204 No Content` (tùy S3-compatible behavior).

Upload xong có thể dùng `public_url` ngay trong `POST /posts`.

---

## Feed

### GET /feed

Lấy danh sách post cho news feed của user đang đăng nhập (có pagination bằng cursor).

**Auth:** Bắt buộc

#### Request
```http
GET /feed?cursor=<cursor>&limit=<n>
Authorization: Bearer <access_token>
```

Query params:
- `limit` (optional): default `10`, max `50`
- `cursor` (optional): cursor pagination do backend trả về

#### Cursor format
Feed dùng cursor dạng:

- Format: `<post_id>:<timestamp>`
- Ví dụ: `1050:1734439200`

Frontend nên treat cursor là **opaque**:
- Lấy `next_cursor` từ response và gửi nguyên chuỗi đó cho request kế tiếp.

#### Response (200 OK)
Backend trả về object feed (không có wrapper data/meta).

```json
{
  "posts": [
    {
      "id": 1050,
      "user_id": 501,
      "caption": "Beautiful sunset!",
      "like_count": 42,
      "comment_count": 5,
      "created_at": "2025-12-02T10:00:00Z",
      "updated_at": "2025-12-02T10:00:00Z",
      "media": [
        {
          "id": 9001,
          "media_url": "https://...",
          "media_type": "image",
          "position": 0
        }
      ],
      "author": {
        "id": 501,
        "username": "johndoe",
        "display_name": "John Doe",
        "avatar_url": "https://...",
        "is_following": true
      }
    }
  ],
  "next_cursor": "1050:1734439200",
  "has_more": true
}
```

Field notes:
- `has_more`: backend hiện set `has_more = (len(posts) == limit)`
- `next_cursor`: chỉ xuất hiện khi `has_more = true`
- `author.is_following`: chỉ đúng vì endpoint này yêu cầu auth (backend check follow status)
- `is_liked`: `true` nếu user hiện tại đã like post này

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `400 BAD_REQUEST`: `limit` không hợp lệ
- `500 INTERNAL_ERROR`: lỗi server

---

## Posts

### POST /posts

Tạo post mới cho user đang đăng nhập.

**Auth:** Bắt buộc

#### Request
```http
POST /posts
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "caption": "Hello world!",
  "media_urls": [
    "https://<public-r2-domain>/posts/<uuid>.jpg",
    "https://<public-r2-domain>/posts/<uuid>.png"
  ]
}
```

Validation:
- `media_urls` bắt buộc và phải có **ít nhất 1 item**
- Tối đa `media_urls.length = 10`
- `caption` optional, max length = **2200**

#### Response (201 Created)
Backend trả về object post (không có wrapper data/meta).

```json
{
  "id": 123,
  "user_id": 1,
  "caption": "Hello world!",
  "like_count": 0,
  "comment_count": 0,
  "created_at": "2025-12-17T10:00:00Z",
  "updated_at": "2025-12-17T10:00:00Z",
  "media": [
    { "id": 999, "media_url": "https://<public>/posts/<uuid>.jpg", "media_type": "image", "position": 0 },
    { "id": 1000, "media_url": "https://<public>/posts/<uuid>.png", "media_type": "image", "position": 1 }
  ],
  "author": {
    "id": 1,
    "username": "alice",
    "display_name": "Alice",
    "avatar_url": "https://<public>/avatars/<uuid>.jpg",
    "is_following": false
  }
}
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `400 BAD_REQUEST` (message thường gặp):
  - "At least one media item is required"
  - "Too many media items (max 10)"
  - "Caption too long (max 2200 characters)"
- `500 INTERNAL_ERROR`: lỗi tạo post

#### Side effects
- Insert `posts` + `post_details` trong transaction
- `users.post_count = post_count + 1` trong transaction
- Publish event `post_created` lên Redis Streams để worker fan-out feed (best-effort; fail publish không làm fail create post)

---

### GET /posts/{id}

Lấy chi tiết 1 post.

**Auth:** Optional

#### Request
```http
GET /posts/123
Authorization: Bearer <access_token> (optional)
```

#### Response (200 OK)
Trả về 1 `Post` (shape tương tự `POST /posts`).

#### Errors
- `400 BAD_REQUEST`: `id` không hợp lệ
- `404 NOT_FOUND`: post không tồn tại hoặc đã soft-delete
- `500 INTERNAL_ERROR`: lỗi server

Ghi chú:
- `is_liked` sẽ được set đúng nếu request có Bearer token (backend check bảng `post_likes`).

---

### DELETE /posts/{id}

Soft-delete post. Chỉ chủ post mới được xóa.

**Auth:** Bắt buộc

#### Request
```http
DELETE /posts/123
Authorization: Bearer <access_token>
```

#### Response (200 OK)
```json
{ "message": "Post deleted successfully" }
```

#### Errors
- `401 UNAUTHORIZED`: thiếu token / token không hợp lệ
- `403 FORBIDDEN`: không phải chủ post
- `404 NOT_FOUND`: post không tồn tại hoặc đã bị xóa

#### Side effects
- Set `posts.deleted_at = NOW()`
- `users.post_count = post_count - 1`
- Publish event `post_deleted` lên Redis Streams để worker remove khỏi feed (best-effort)

---

### GET /users/{id}/posts

Lấy danh sách thumbnail post của 1 user (grid profile).

**Auth:** Optional

#### Request
```http
GET /users/1/posts?cursor=<cursor>&limit=12
Authorization: Bearer <access_token> (optional)
```

Query params:
- `limit` (optional): default `12`, max `36`
- `cursor` (optional): chuỗi cursor do backend trả về

#### Cursor format
Cursor format giờ đã thống nhất với `/feed`:

- Format: `<post_id>:<unix_timestamp>`
- Ví dụ: `123:1734439200`

Frontend nên treat cursor là **opaque**:
- Lấy `next_cursor` từ response và gửi nguyên chuỗi đó cho request kế tiếp.

#### Response (200 OK)
```json
{
  "posts": [
    {
      "id": 123,
      "thumbnail_url": "https://<public>/posts/<uuid>.jpg",
      "media_count": 2
    }
  ],
  "next_cursor": "123:1734439200",
  "has_more": true
}
```

Field notes:
- `has_more`: `true` nếu còn posts để load
- `next_cursor`: chỉ xuất hiện khi `has_more = true`

#### Errors
- `400 BAD_REQUEST`: user id không hợp lệ hoặc limit không hợp lệ
- `500 INTERNAL_ERROR`: lỗi server

---

## Ghi chú quan trọng / giới hạn hiện tại

1) **Like / comment endpoints chưa được implement.**
- Hiện router/service chưa có các endpoint kiểu:
  - `POST /posts/:id/like`, `DELETE /posts/:id/like`
  - `POST /posts/:id/comments`, `GET /posts/:id/comments`
- Frontend không nên tích hợp các tính năng này cho tới khi backend implement.

2) **`media_type` hiện luôn là "image".**
- Khi create post, backend đang set mặc định "image" cho mọi media. Video chưa hỗ trợ.

3) **Chưa có cleanup object R2 khi xóa post.**
- Xóa post sẽ soft-delete trong DB nhưng không xóa file trên R2 (đã thống nhất skip tạm).

