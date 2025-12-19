# Notifications & Push Notifications API

Tài liệu này mô tả **toàn bộ các endpoint liên quan đến Notifications** trong backend, bao gồm cả flow đăng ký **device token** cho push notifications (FCM).

Mục tiêu: đủ rõ ràng để team frontend có thể implement theo.

---

## Mục lục
1. [Authentication](#authentication)
2. [Data Models (TypeScript types)](#data-models-typescript-types)
3. [Notifications](#notifications)
   - [GET /notifications](#get-notifications)
   - [PATCH /notifications/read](#patch-notificationsread)
   - [POST /notifications/read-all](#post-notificationsread-all)
   - [GET /notifications/unread-count](#get-notificationsunread-count)
4. [Device Token (Push Notifications)](#device-token-push-notifications)
   - [POST /devices/token](#post-devicestoken)
   - [DELETE /devices/token](#delete-devicestoken)
5. [Push Notification Flow](#push-notification-flow)
6. [Frontend Implementation Notes](#frontend-implementation-notes)

---

## Authentication

Tất cả endpoint đều yêu cầu access token:

```http
Authorization: Bearer <access_token>
```

### Format lỗi (chuẩn chung)
```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Thông điệp lỗi"
  }
}
```

---

## Data Models (TypeScript types)

### Notification (follow notification - không aggregated)
```ts
export type Notification = {
  id: number;
  actor_id: number;
  type: "follow" | "like" | "comment";
  post_id?: number;       // null for follow notifications
  comment_id?: number;    // only for comment notifications
  is_read: boolean;
  created_at: string;     // ISO string
  actor?: UserSummary;    // Who triggered the notification
};
```

### AggregatedNotification (likes/comments grouped by post)
```ts
export type AggregatedNotification = {
  type: "like" | "comment";
  post_id?: number;                // For navigation to post
  actors: UserSummary[];           // First 2-3 actors (for "user1 and X others")
  total_count: number;             // Total number of actors
  latest_at: string;               // Most recent activity
  is_read: boolean;                // True if ALL in group are read
};
```

### NotificationListResponse
```ts
export type NotificationListResponse = {
  follows: Notification[];              // Individual follow notifications
  aggregated: AggregatedNotification[]; // Grouped likes/comments
  unread_count: number;                 // For badge display
};
```

### UserSummary
```ts
export type UserSummary = {
  id: number;
  username: string;
  display_name: string | null;
  avatar_url: string | null;
};
```

---

## Notifications

### GET /notifications

Lấy danh sách tất cả notifications của user đang đăng nhập.

**Auth:** Bắt buộc

#### Request
```http
GET /notifications?limit=20
Authorization: Bearer <access_token>
```

Query params:
- `limit` (optional): default `20`, max `50`

#### Response (200 OK)
```json
{
  "follows": [
    {
      "id": 123,
      "actor_id": 456,
      "type": "follow",
      "is_read": false,
      "created_at": "2025-12-20T10:00:00Z",
      "actor": {
        "id": 456,
        "username": "john_doe",
        "display_name": "John Doe",
        "avatar_url": "https://..."
      }
    }
  ],
  "aggregated": [
    {
      "type": "like",
      "post_id": 789,
      "actors": [
        { "id": 101, "username": "alice", "display_name": "Alice", "avatar_url": "https://..." },
        { "id": 102, "username": "bob", "display_name": "Bob", "avatar_url": null }
      ],
      "total_count": 5,
      "latest_at": "2025-12-20T09:30:00Z",
      "is_read": false
    }
  ],
  "unread_count": 3
}
```

**UI Display Notes:**
- **Follow**: Hiển thị từng notification riêng lẻ: "john_doe started following you"
- **Aggregated Likes**: "alice and 4 others liked your post"
- **Aggregated Comments**: "bob and 2 others commented on your post"
- Click vào like/comment → navigate đến `post_id`
- Click vào follow → navigate đến profile của `actor_id`

---

### PATCH /notifications/read

Đánh dấu các notifications cụ thể đã đọc.

**Auth:** Bắt buộc

#### Request
```http
PATCH /notifications/read
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "notification_ids": [123, 124, 125]
}
```

#### Response (200 OK)
```json
{
  "message": "Notifications marked as read"
}
```

#### Errors
- `400 BAD_REQUEST`: `notification_ids` is required

---

### POST /notifications/read-all

Đánh dấu TẤT CẢ notifications đã đọc (ví dụ khi user mở notification screen).

**Auth:** Bắt buộc

#### Request
```http
POST /notifications/read-all
Authorization: Bearer <access_token>
```

#### Response (200 OK)
```json
{
  "message": "All notifications marked as read"
}
```

---

### GET /notifications/unread-count

Lấy số lượng unread notifications (để hiển thị badge trên app icon).

**Auth:** Bắt buộc

#### Request
```http
GET /notifications/unread-count
Authorization: Bearer <access_token>
```

#### Response (200 OK)
```json
{
  "unread_count": 5
}
```

---

## Device Token (Push Notifications)

### POST /devices/token

Đăng ký FCM device token để nhận push notifications.

**Khi nào gọi:**
- Mỗi lần app launch (token có thể refresh bất cứ lúc nào)
- Sau khi login thành công
- Khi FCM SDK báo token mới (onTokenRefresh callback)

**Auth:** Bắt buộc

#### Request
```http
POST /devices/token
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "token": "fcm-device-token-string-here",
  "platform": "ios"
}
```

Fields:
- `token` (required): FCM token từ Firebase SDK
- `platform` (optional): `"ios"` hoặc `"android"`, default `"android"`

#### Response (200 OK)
```json
{
  "message": "Device token registered"
}
```

**Implementation Notes:**
- Backend dùng UPSERT - nếu token đã tồn tại, chỉ update timestamp
- Safe để gọi nhiều lần không gây duplicate

---

### DELETE /devices/token

Xóa device token (khi user logout).

**Khi nào gọi:**
- Khi user bấm logout
- Trước khi clear local storage

#### Request
```http
DELETE /devices/token
Content-Type: application/json
```

```json
{
  "token": "fcm-device-token-string-here"
}
```

**Note:** Có thể gọi mà không cần auth header (trong trường hợp token đã hết hạn).

#### Response (200 OK)
```json
{
  "message": "Device token removed"
}
```

---

## Push Notification Flow

### Khi nào user nhận push notification?

| Trigger | Notification |
|---------|--------------|
| User B follows User A | A nhận: "B started following you" |
| User B likes A's post | A nhận: "B liked your post" |
| User B comments on A's post | A nhận: "B commented on your post" |

**Lưu ý:** User KHÔNG nhận notification cho actions của chính họ (like post của mình, comment post của mình).

### Push Notification Payload

Push notifications sẽ hiển thị:
- **Title**: "New Follower", "New Like", "New Comment"
- **Body**: "username started following you", "username liked your post", etc.

---

## Frontend Implementation Notes

### 1. FCM Setup (React Native)

```bash
npm install @react-native-firebase/app @react-native-firebase/messaging
```

### 2. Register Token Flow

```ts
import messaging from '@react-native-firebase/messaging';

// On app launch (after login)
async function registerDeviceToken() {
  // Request permission
  const authStatus = await messaging().requestPermission();
  const enabled = authStatus === messaging.AuthorizationStatus.AUTHORIZED;
  
  if (enabled) {
    // Get FCM token
    const token = await messaging().getToken();
    
    // Send to backend
    await api.post('/devices/token', {
      token,
      platform: Platform.OS // 'ios' or 'android'
    });
  }
}

// Listen for token refresh
messaging().onTokenRefresh(async (token) => {
  await api.post('/devices/token', { token, platform: Platform.OS });
});
```

### 3. Polling for In-App Notifications

```ts
// Poll every 30 seconds when app is in foreground
useEffect(() => {
  const interval = setInterval(() => {
    fetchNotifications();
  }, 30000);
  
  return () => clearInterval(interval);
}, []);

// Also fetch on notification screen focus
useFocusEffect(() => {
  fetchNotifications();
});
```

### 4. Badge Count

```ts
// Update app icon badge
import PushNotificationIOS from '@react-native-community/push-notification-ios';

const updateBadge = (count: number) => {
  if (Platform.OS === 'ios') {
    PushNotificationIOS.setApplicationIconBadgeNumber(count);
  }
  // Android badge handling varies by launcher
};
```

### 5. Logout Flow

```ts
async function logout() {
  const token = await messaging().getToken();
  
  // Remove device token from backend
  await api.delete('/devices/token', { data: { token } });
  
  // Clear local auth state
  await clearAuthTokens();
  
  // Navigate to login
  navigation.reset({ routes: [{ name: 'Login' }] });
}
```

---

## Ghi chú quan trọng / giới hạn hiện tại

1. **FCM credentials chưa được configure** - Backend hiện đang khởi tạo FCM client với `nil`. Cần set Firebase credentials trong `.env` để push hoạt động.

2. **Aggregation logic** - Likes/comments được group theo `post_id`. Không có time-window (tất cả likes vào cùng 1 post đều group chung).

3. **Multi-device support** - Backend hỗ trợ nhiều thiết bị cùng 1 user. Push sẽ gửi đến TẤT CẢ devices đã đăng ký.

4. **Read status** - Aggregated notification `is_read = true` chỉ khi TẤT CẢ notifications trong group đã đọc.
