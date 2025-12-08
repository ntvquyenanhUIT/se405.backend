# Tài Liệu Authentication & Authorization Flow

## Mục Lục
1. [Tổng Quan](#tổng-quan)
2. [Cơ Chế Token](#cơ-chế-token)
3. [API Endpoints](#api-endpoints)
4. [Error Handling](#error-handling)
5. [Flow Diagrams](#flow-diagrams)
6. [FAQ](#faq)

---

## Tổng Quan

### Kiến Trúc Authentication
Hệ thống sử dụng **JWT-based authentication** với **refresh token rotation** để bảo mật:

- **Access Token**: JWT token có thời hạn ngắn (thời gian được set trong config) dùng để authenticate các request
- **Refresh Token**: UUID token được hash và lưu trong database, có thời hạn dài hơn, dùng để lấy access token mới
- **Token Rotation**: Mỗi lần refresh, cả access token và refresh token đều được tạo mới, token cũ bị revoke
- **Reuse Detection**: Hệ thống phát hiện việc reuse refresh token (dấu hiệu của token theft) và revoke toàn bộ token của user

### Authentication Methods
Backend support 2 cách gửi access token:

1. **Authorization Header** (khuyên dùng cho mobile):
   ```
   Authorization: Bearer <access_token>
   ```

2. **Cookie** (dành cho web browser):
   ```
   Cookie: access_token=<access_token>
   ```

**QUAN TRỌNG**: Frontend mobile phải dùng Authorization header!

---

## Cơ Chế Token

### Access Token Structure
Access token là JWT với payload:
```json
{
  "user_id": 123,
  "exp": 1702345678,  // Expiration timestamp
  "iat": 1702342078   // Issued at timestamp
}
```

### Token Lifecycle
```
1. User đăng nhập → nhận access_token + refresh_token
2. Dùng access_token cho các API request
3. Access_token hết hạn → gọi /auth/refresh với refresh_token
4. Nhận cặp token mới → refresh_token cũ bị revoke tự động
5. Lặp lại bước 2-4
```

### Token Storage - QUAN TRỌNG!
**Frontend PHẢI làm đúng như sau:**

| Token Type | Nơi Lưu | Lý Do |
|------------|---------|-------|
| Access Token | Memory (state/store) hoặc SecureStorage | Token này được dùng liên tục, cần access nhanh |
| Refresh Token | SecureStorage (React Native) / HttpOnly Cookie (Web) | Token này sensitive hơn, cần bảo mật cao |

**KHÔNG BAO GIỜ:**
- ❌ Lưu token trong localStorage (dễ bị XSS)
- ❌ Lưu token trong AsyncStorage không encrypt (dễ bị đọc)
- ❌ Log token ra console trong production
- ❌ Gửi token trong URL parameters

**NÊN:**
- ✅ Dùng React Native SecureStore/Keychain cho mobile
- ✅ Clear tokens khi user logout
- ✅ Implement auto-refresh trước khi token expire

---

## API Endpoints

### 1. Đăng Ký (Register)

#### Request
```http
POST /auth/register
Content-Type: multipart/form-data

FormData:
  username: string (required) - Tên đăng nhập, phải unique
  password: string (required) - Mật khẩu (backend sẽ hash)
  display_name: string (optional) - Tên hiển thị
  avatar: File (optional) - Ảnh đại diện (jpeg, png, gif, webp, max 5MB)
```

**LƯU Ý FRONTEND:**
- Content-Type PHẢI là `multipart/form-data` (vì có upload file)
- Nếu không upload avatar, backend tự động dùng default avatar
- Username phải được trim() trước khi gửi
- File size PHẢI validate ở client trước: max 5MB
- File type PHẢI validate: chỉ jpeg, png, gif, webp

#### Response Success (201 Created)
```json
{
  "id": 1,
  "username": "nguyenvana",
  "display_name": "Nguyễn Van A",
  "avatar_url": "https://cdn.example.com/avatars/abc123.jpg",
  "bio": null,
  "is_new_user": true,
  "follower_count": 0,
  "following_count": 0,
  "post_count": 0,
  "created_at": "2024-12-07T10:30:00Z",
  "updated_at": "2024-12-07T10:30:00Z"
}
```

**Frontend cần làm sau khi nhận response:**
1. Lưu thông tin user vào state/store
2. Navigate đến màn hình login (hoặc tự động login - xem phần login)
3. Show thông báo "Đăng ký thành công"

#### Response Error

**Username đã tồn tại (409 Conflict):**
```json
{
  "error": {
    "code": "CONFLICT",
    "message": "Username already exists"
  }
}
```

**File quá lớn (400 Bad Request):**
```json
{
  "error": {
    "code": "FILE_TOO_LARGE",
    "message": "Avatar exceeds 5MB limit"
  }
}
```

**File type không hợp lệ (400 Bad Request):**
```json
{
  "error": {
    "code": "INVALID_IMAGE_TYPE",
    "message": "Unsupported image type. Allowed: jpeg, png, gif, webp"
  }
}
```

**Request không hợp lệ (400 Bad Request):**
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Username is required" // hoặc message khác
  }
}
```

**Frontend cần handle:**
- Show error message từ `error.message` cho user
- Nếu code là `CONFLICT`: focus vào username field, suggest thử username khác
- Nếu code là `FILE_TOO_LARGE`: show thông báo compress/chọn ảnh khác
- Nếu code là `INVALID_IMAGE_TYPE`: show list file types được phép

---

### 2. Đăng Nhập (Login)

#### Request
```http
POST /auth/login
Content-Type: application/json

{
  "username": "nguyenvana",
  "password": "securepassword123"
}
```

**LƯU Ý FRONTEND:**
- Username và password KHÔNG được empty
- Backend sẽ tự extract `User-Agent` header và IP address để track device
- Không cần gửi device info, backend tự lấy

#### Response Success (200 OK)
```json
{
  "user": {
    "id": 1,
    "username": "nguyenvana",
    "display_name": "Nguyễn Van A",
    "avatar_url": "https://cdn.example.com/avatars/abc123.jpg",
    "bio": "Hello world",
    "is_new_user": false,
    "follower_count": 150,
    "following_count": 200,
    "post_count": 45,
    "created_at": "2024-12-07T10:30:00Z",
    "updated_at": "2024-12-07T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "a3d5e8f1-9b2c-4a6e-8d7f-1c3b5e7a9f2d",
  "expires_in": 3600
}
```

**Frontend PHẢI làm ngay sau khi nhận response:**
1. **Lưu tokens ngay lập tức:**
   ```javascript
   // Pseudo-code
   await SecureStorage.setItem('refresh_token', response.refresh_token)
   authStore.setAccessToken(response.access_token) // Lưu vào memory/state
   ```

2. **Lưu thông tin user:**
   ```javascript
   userStore.setUser(response.user)
   ```

3. **Setup auto-refresh timer:**
   ```javascript
   // expires_in là số giây, set timer refresh trước 30s-1 phút
   const refreshTime = (response.expires_in - 60) * 1000
   setTimeout(() => {
     refreshAccessToken()
   }, refreshTime)
   ```

4. **Navigate đến home screen**

5. **Check `is_new_user`:**
   ```javascript
   if (response.user.is_new_user) {
     // Navigate đến onboarding flow
   } else {
     // Navigate đến home feed
   }
   ```

#### Response Error

**Sai username/password (401 Unauthorized):**
```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid username or password"
  }
}
```

**Request không hợp lệ (400 Bad Request):**
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Username is required" // hoặc "Password is required"
  }
}
```

**Frontend cần handle:**
- Show error message từ `error.message`
- Nếu 401: focus vào password field, có thể show "Quên mật khẩu?"
- Nếu nhiều lần 401: có thể suggest "Bạn có muốn reset password không?"

---

### 3. Refresh Token

#### Request
```http
POST /auth/refresh
Content-Type: application/json

{
  "refresh_token": "a3d5e8f1-9b2c-4a6e-8d7f-1c3b5e7a9f2d"
}
```

**KHI NÀO GỌI API NÀY:**
- Trước khi access token expire (setup timer như đã nói ở phần login)
- Khi nhận response 401 với code `TOKEN_EXPIRED` từ bất kỳ API nào
- Khi app khởi động (nếu có refresh token) để check xem còn valid không

**LƯU Ý FRONTEND:**
- Backend tự lấy `User-Agent` và IP address từ request headers
- Phải lấy refresh_token từ SecureStorage
- Phải implement retry logic (xem bên dưới)

#### Response Success (200 OK)
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "b4e6f9g2-0c3d-5b7f-9e8g-2d4c6f8b0g3e",
  "expires_in": 3600
}
```

**Frontend PHẢI làm ngay:**
1. **Update cả 2 tokens:**
   ```javascript
   await SecureStorage.setItem('refresh_token', response.refresh_token)
   authStore.setAccessToken(response.access_token)
   ```

2. **Setup lại timer:**
   ```javascript
   const refreshTime = (response.expires_in - 60) * 1000
   setTimeout(() => {
     refreshAccessToken()
   }, refreshTime)
   ```

3. **Retry request bị fail (nếu có):**
   ```javascript
   // Nếu đang retry sau khi nhận 401
   return retryOriginalRequest(originalConfig)
   ```

#### Response Error

**Refresh token không tồn tại/không hợp lệ (401 Unauthorized):**
```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid refresh token"
  }
}
```

**Refresh token đã expire (401 Unauthorized):**
```json
{
  "error": {
    "code": "TOKEN_EXPIRED",
    "message": "Refresh token has expired"
  }
}
```

**PHÁT HIỆN REUSE - QUAN TRỌNG! (401 Unauthorized):**
```json
{
  "error": {
    "code": "TOKEN_REUSED",
    "message": "Refresh token reuse detected. Please login again."
  }
}
```

**Frontend PHẢI handle từng case:**

```javascript
if (error.response?.status === 401) {
  const errorCode = error.response.data?.error?.code
  
  switch(errorCode) {
    case 'UNAUTHORIZED':
    case 'TOKEN_EXPIRED':
      // Refresh token đã hết hạn hoặc không hợp lệ
      // Logout user và navigate về login screen
      await logout(false) // false = không gọi logout API
      navigateToLogin()
      showMessage("Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.")
      break
      
    case 'TOKEN_REUSED':
      // Có thể bị đánh cắp token! Log user ra khỏi tất cả devices
      await logout(false)
      navigateToLogin()
      showAlert({
        title: "Cảnh báo bảo mật",
        message: "Phát hiện hoạt động bất thường. Vui lòng đăng nhập lại và đổi mật khẩu.",
        critical: true
      })
      break
  }
}
```

---

### 4. Đăng Xuất (Logout)

#### Request
```http
POST /auth/logout
Content-Type: application/json
Authorization: Bearer <access_token>

{
  "refresh_token": "a3d5e8f1-9b2c-4a6e-8d7f-1c3b5e7a9f2d"
}
```

**LƯU Ý FRONTEND:**
- Cần gửi cả access token (header) và refresh token (body)
- Refresh token sẽ bị revoke ở backend
- Access token vẫn valid cho đến khi expire (nhưng frontend nên xóa ngay)

#### Response Success (200 OK)
```json
{
  "message": "Logged out successfully"
}
```

**Frontend PHẢI làm sau khi nhận response:**
```javascript
async function logout() {
  try {
    // 1. Lấy refresh token
    const refreshToken = await SecureStorage.getItem('refresh_token')
    
    // 2. Gọi API logout (có thể skip nếu offline)
    if (isOnline && refreshToken) {
      await api.post('/auth/logout', { refresh_token: refreshToken })
    }
  } catch (error) {
    // Ignore error, vẫn logout ở client
  } finally {
    // 3. Clear tất cả auth data
    await SecureStorage.removeItem('refresh_token')
    authStore.clearAccessToken()
    userStore.clearUser()
    
    // 4. Cancel auto-refresh timer
    if (refreshTimer) {
      clearTimeout(refreshTimer)
    }
    
    // 5. Navigate về login
    navigateToLogin()
  }
}
```

#### Response Error

**Request không hợp lệ (400 Bad Request):**
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Refresh token is required"
  }
}
```

**Refresh token không tồn tại (200 OK - vẫn success!):**
```json
{
  "message": "Logged out successfully"
}
```

**LƯU Ý:**
- Backend trả về success ngay cả khi refresh token không tồn tại
- Frontend PHẢI clear tokens dù API call có lỗi hay không

---

## Error Handling

### Error Response Format
**TẤT CẢ** error responses đều có format:
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

### Common Error Codes

| HTTP Status | Error Code | Ý Nghĩa | Frontend Action |
|-------------|-----------|---------|-----------------|
| 400 | `BAD_REQUEST` | Request không hợp lệ (thiếu field, sai format) | Show message, highlight field lỗi |
| 400 | `FILE_TOO_LARGE` | File upload quá 5MB | Show message, suggest compress/chọn file khác |
| 400 | `INVALID_IMAGE_TYPE` | File type không phải image hợp lệ | Show message, list file types được phép |
| 401 | `UNAUTHORIZED` | Không có token hoặc token không hợp lệ | Logout và navigate về login |
| 401 | `TOKEN_EXPIRED` | Access token đã hết hạn | Tự động refresh token |
| 401 | `TOKEN_INVALID` | Token malformed hoặc signature sai | Logout và navigate về login |
| 401 | `TOKEN_REUSED` | Phát hiện reuse refresh token | Logout, show security warning |
| 404 | `NOT_FOUND` | Resource không tồn tại | Show message |
| 409 | `CONFLICT` | Dữ liệu bị conflict (username đã tồn tại) | Show message, suggest alternative |
| 500 | `INTERNAL_ERROR` | Server error | Show generic error, có retry button |

### Axios Interceptor Example

Frontend NÊN implement interceptor để handle auth errors globally:

```javascript
import axios from 'axios'
import { getAccessToken, getRefreshToken, setAccessToken, setRefreshToken, clearAuth } from './auth'

// Flag để tránh multiple refresh calls
let isRefreshing = false
let failedQueue = []

const processQueue = (error, token = null) => {
  failedQueue.forEach(prom => {
    if (error) {
      prom.reject(error)
    } else {
      prom.resolve(token)
    }
  })
  failedQueue = []
}

// Request interceptor: Thêm access token vào header
axios.interceptors.request.use(
  config => {
    const token = getAccessToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  error => Promise.reject(error)
)

// Response interceptor: Handle token expiration
axios.interceptors.response.use(
  response => response,
  async error => {
    const originalRequest = error.config

    // Nếu không phải 401 hoặc đã retry rồi, reject luôn
    if (error.response?.status !== 401 || originalRequest._retry) {
      return Promise.reject(error)
    }

    const errorCode = error.response?.data?.error?.code

    // Handle các loại 401 khác nhau
    if (errorCode === 'TOKEN_EXPIRED') {
      // Access token hết hạn, cần refresh

      if (isRefreshing) {
        // Đang refresh rồi, queue request này lại
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject })
        })
          .then(token => {
            originalRequest.headers.Authorization = `Bearer ${token}`
            return axios(originalRequest)
          })
          .catch(err => Promise.reject(err))
      }

      originalRequest._retry = true
      isRefreshing = true

      try {
        const refreshToken = await getRefreshToken()
        
        if (!refreshToken) {
          throw new Error('No refresh token')
        }

        // Gọi refresh API
        const response = await axios.post('/auth/refresh', {
          refresh_token: refreshToken
        })

        const { access_token, refresh_token: newRefreshToken } = response.data

        // Save tokens mới
        await setAccessToken(access_token)
        await setRefreshToken(newRefreshToken)

        // Update header cho request hiện tại
        originalRequest.headers.Authorization = `Bearer ${access_token}`

        // Process queue
        processQueue(null, access_token)

        // Retry request gốc
        return axios(originalRequest)

      } catch (refreshError) {
        // Refresh thất bại
        processQueue(refreshError, null)
        await clearAuth()
        // Navigate to login screen
        // navigation.navigate('Login') - tùy theo routing library
        return Promise.reject(refreshError)
        
      } finally {
        isRefreshing = false
      }

    } else if (errorCode === 'TOKEN_REUSED') {
      // Phát hiện token bị reuse - NGHIÊM TRỌNG!
      await clearAuth()
      // Show security alert
      Alert.alert(
        'Cảnh báo bảo mật',
        'Phát hiện hoạt động bất thường. Vui lòng đăng nhập lại và đổi mật khẩu.',
        [{ text: 'OK', onPress: () => {
          // Navigate to login
        }}]
      )
      return Promise.reject(error)

    } else {
      // Các lỗi 401 khác (UNAUTHORIZED, TOKEN_INVALID)
      await clearAuth()
      // Navigate to login
      return Promise.reject(error)
    }
  }
)

export default axios
```

---

## Flow Diagrams

### 1. Login Flow
```
User nhập credentials
        ↓
Frontend validate input
        ↓
POST /auth/login
        ↓
Backend verify credentials
        ↓
Backend generate access token (JWT)
        ↓
Backend create & hash refresh token
        ↓
Backend save refresh token to database
        ↓
Backend return: user + access_token + refresh_token
        ↓
Frontend save refresh_token to SecureStorage
        ↓
Frontend save access_token to memory
        ↓
Frontend save user info to store
        ↓
Frontend setup auto-refresh timer
        ↓
Navigate to home screen
```

### 2. API Request với Authentication
```
User action (e.g., get feed)
        ↓
Frontend get access_token from memory
        ↓
Frontend add "Authorization: Bearer <token>" header
        ↓
Send API request
        ↓
Backend middleware extract token
        ↓
Backend verify JWT signature
        ↓
Backend check expiration
        ↓
Backend extract user_id from token
        ↓
Backend add user_id to request context
        ↓
Handler process request
        ↓
Return response
```

### 3. Token Refresh Flow
```
Timer trigger hoặc nhận 401 TOKEN_EXPIRED
        ↓
Frontend get refresh_token from SecureStorage
        ↓
POST /auth/refresh với refresh_token
        ↓
Backend hash refresh_token
        ↓
Backend tìm token trong database
        ↓
Backend check: revoked? expired? reused?
        ↓
Backend generate new token pair
        ↓
Backend revoke old refresh token
        ↓
Backend save new refresh token
        ↓
Return new access_token + refresh_token
        ↓
Frontend save tokens
        ↓
Frontend setup new auto-refresh timer
        ↓
Frontend retry failed request (nếu có)
```

### 4. Token Reuse Detection Flow
```
Attacker dùng refresh token đã bị revoke
        ↓
POST /auth/refresh
        ↓
Backend tìm thấy token trong database
        ↓
Backend check: token.revoked_at != null
        ↓
Backend detect REUSE
        ↓
Backend revoke TẤT CẢ tokens của user
        ↓
Return 401 TOKEN_REUSED
        ↓
Frontend logout user
        ↓
Frontend show security warning
        ↓
Navigate to login screen
```

### 5. Logout Flow
```
User click logout
        ↓
Frontend get refresh_token from SecureStorage
        ↓
POST /auth/logout với refresh_token
        ↓
Backend revoke refresh token in database
        ↓
Return success
        ↓
Frontend clear refresh_token from SecureStorage
        ↓
Frontend clear access_token from memory
        ↓
Frontend clear user info from store
        ↓
Frontend cancel auto-refresh timer
        ↓
Navigate to login screen
```

---

## FAQ



### Q: Tại sao phải dùng cả access token và refresh token?
**A:** 
- **Access token** có thời hạn ngắn (vd: 1 giờ), gửi với mọi request → nếu bị đánh cắp, chỉ sử dụng được trong thời gian ngắn
- **Refresh token** có thời hạn dài (vd: 30 ngày), chỉ gửi khi refresh → ít bị expose hơn
- Kết hợp cả 2 = security cao + UX tốt (không phải login liên tục)

### Q: Tại sao phải revoke refresh token cũ khi refresh?
**A:** **Token rotation** - mỗi refresh token chỉ dùng được 1 lần. Nếu token bị reuse → phát hiện được token theft và revoke toàn bộ sessions của user.

### Q: Khi nào nên refresh token?
**A:** 
1. **Proactive**: Setup timer refresh trước 30-60s khi token expire
2. **Reactive**: Khi nhận 401 TOKEN_EXPIRED từ API
3. **On app startup**: Check refresh token còn valid không

### Q: Làm sao để test token reuse detection?
**A:**
1. Login → lưu lại refresh_token_1
2. Gọi /auth/refresh với refresh_token_1 → nhận refresh_token_2
3. Gọi /auth/refresh lại với refresh_token_1 (token cũ đã bị revoke)
4. Backend sẽ trả về 401 TOKEN_REUSED

### Q: Có nên lưu access token vào SecureStorage không?
**A:** Không nhất thiết. Access token được dùng liên tục nên lưu trong memory (state/store) là đủ. Chỉ refresh token mới cần SecureStorage vì nó sensitive và tồn tại lâu.

### Q: Điều gì xảy ra nếu user có nhiều devices?
**A:** Mỗi device có 1 refresh token riêng. Khi logout all devices, tất cả refresh tokens đều bị revoke. User phải login lại trên tất cả devices.

### Q: Backend có rate limiting không?
**A:** Tài liệu này không đề cập, nhưng frontend nên implement:
- Debounce login button
- Limit số lần retry
- Exponential backoff cho failed requests

### Q: Nếu app bị kill khi đang refresh token thì sao?
**A:** 
- Refresh token vẫn an toàn trong SecureStorage
- Lần khởi động sau, app sẽ refresh lại
- Nếu cả 2 tokens đều expire → user phải login lại

---

## Liên Hệ

Nếu có thắc mắc hoặc phát hiện bug, liên hệ:
- Backend Team Lead: [Your Name]
- Email: [your.email@example.com]
- Slack: #backend-support

**LƯU Ý:** Đọc kỹ tài liệu này trước khi hỏi! Hầu hết câu hỏi đã có trong FAQ và examples.

---

**Document Version:** 1.0  
**Last Updated:** December 7, 2024  
**Author:** Backend Team
