# Notifications & Push Notifications API

Tài liệu này mô tả **toàn bộ các endpoint liên quan đến Notifications** trong backend, sử dụng **Expo Push** cho push notifications.

Mục tiêu: đủ rõ ràng để team frontend (React Native + Expo) có thể implement theo.

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

Đăng ký **Expo Push Token** để nhận push notifications.

**Khi nào gọi:**
- Mỗi lần app launch (token có thể refresh bất cứ lúc nào)
- Sau khi login thành công
- Khi Expo SDK báo token mới

**Auth:** Bắt buộc

#### Request
```http
POST /devices/token
Content-Type: application/json
Authorization: Bearer <access_token>
```

```json
{
  "token": "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]",
  "platform": "expo"
}
```

Fields:
- `token` (required): Expo Push Token từ `expo-notifications`
- `platform` (optional): `"expo"`, `"ios"`, or `"android"`, default `"expo"`

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
  "token": "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]"
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
- **Data**: `{ type, actor_id, post_id }` cho navigation

---

## Frontend Implementation Notes (Expo)

### 1. Setup (Expo + React Native)

```bash
npx expo install expo-notifications expo-device
```

### 2. Register Token Flow

```ts
import * as Notifications from 'expo-notifications';
import * as Device from 'expo-device';
import { Platform } from 'react-native';

// On app launch (after login)
async function registerForPushNotifications() {
  if (!Device.isDevice) {
    console.log('Push notifications only work on physical devices');
    return;
  }

  // Request permission
  const { status: existingStatus } = await Notifications.getPermissionsAsync();
  let finalStatus = existingStatus;
  
  if (existingStatus !== 'granted') {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }
  
  if (finalStatus !== 'granted') {
    console.log('Failed to get push token permissions');
    return;
  }

  // Get Expo push token
  const token = (await Notifications.getExpoPushTokenAsync()).data;
  console.log('Expo Push Token:', token);
  // Token looks like: "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]"

  // Send to backend
  await api.post('/devices/token', {
    token,
    platform: 'expo'
  });

  // For Android, set notification channel
  if (Platform.OS === 'android') {
    Notifications.setNotificationChannelAsync('default', {
      name: 'default',
      importance: Notifications.AndroidImportance.MAX,
    });
  }
}
```

### 3. Handle Incoming Notifications

```ts
import * as Notifications from 'expo-notifications';

// Configure how notifications are displayed when app is in foreground
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});

// Listen for notifications when app is open
useEffect(() => {
  const subscription = Notifications.addNotificationReceivedListener(notification => {
    console.log('Notification received:', notification);
    // Optionally refresh notification list
    fetchNotifications();
  });

  return () => subscription.remove();
}, []);

// Handle notification tap (deep linking)
useEffect(() => {
  const subscription = Notifications.addNotificationResponseReceivedListener(response => {
    const data = response.notification.request.content.data;
    
    if (data.type === 'follow') {
      navigation.navigate('Profile', { userId: data.actor_id });
    } else if (data.post_id) {
      navigation.navigate('Post', { postId: data.post_id });
    }
  });

  return () => subscription.remove();
}, []);
```

### 4. Polling for In-App Notifications

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

### 5. Logout Flow

```ts
import * as Notifications from 'expo-notifications';

async function logout() {
  const token = (await Notifications.getExpoPushTokenAsync()).data;
  
  // Remove device token from backend
  await api.delete('/devices/token', { data: { token } });
  
  // Clear local auth state
  await clearAuthTokens();
  
  // Navigate to login
  navigation.reset({ routes: [{ name: 'Login' }] });
}
```

---

## Ghi chú quan trọng

1. **Expo Push - No setup required!** - Backend sử dụng Expo Push API, không cần Firebase hay Apple Developer account. Works with Expo Go!

2. **Physical device required** - Push notifications không hoạt động trên simulator/emulator.

3. **Aggregation logic** - Likes/comments được group theo `post_id`. Không có time-window (tất cả likes vào cùng 1 post đều group chung).

4. **Multi-device support** - Backend hỗ trợ nhiều thiết bị cùng 1 user. Push sẽ gửi đến TẤT CẢ devices đã đăng ký.

5. **Read status** - Aggregated notification `is_read = true` chỉ khi TẤT CẢ notifications trong group đã đọc.
