# Iamstagram Backend

Backend service cho ứng dụng Iamstagram.

## Yêu cầu tiên quyết (Prerequisites)

Trước khi chạy ứng dụng, hãy đảm bảo bạn đã cài đặt các thành phần sau:

*   **Windows Subsystem for Linux (WSL)**:  Bắt buộc để chạy Redis trên Windows.
    *   [Hướng dẫn cài đặt](https://learn.microsoft.com/en-us/windows/wsl/install)
*   **Redis**: Bắt buộc cho caching và quản lý session.
    *   [Hướng dẫn cài đặt cho Linux/WSL](https://redis.io/docs/latest/operate/oss_and_stack/install/archive/install-redis/install-redis-on-linux/)
*   **Go**: Phiên bản 1.24 hoặc cao hơn.
*   **PostgreSQL**: Cơ sở dữ liệu chính.

## Cài đặt (Installation)

1.  **Cài đặt WSL**: Làm theo hướng dẫn ở link trên để cài WSL cho máy Windows của bạn.
2.  **Cài đặt Redis**: Cài đặt Redis trong môi trường WSL theo hướng dẫn ở link trên.
3.  **Clone Repository**:
    ```bash
    git clone <repository-url>
    cd backend
    ```

## Cấu hình Database

Bạn có 2 lựa chọn để cấu hình database:

### Lựa chọn 1: Sử dụng Remote Database (Khuyên dùng)
Liên hệ trực tiếp với mình (tác giả) để nhận file `.env` chứa cấu hình kết nối tới database remote đã có sẵn data.

### Lựa chọn 2: Tự setup Database cá nhân
Nếu bạn muốn chạy database riêng, hãy làm theo các bước sau:
1.  Cài đặt PostgreSQL trên máy của bạn.
2.  Tạo một database mới (ví dụ: `iamstagram`).
3.  Cấu hình file `.env` trỏ tới database local của bạn.
4.  Chạy các file migration trong thư mục `migrations` để khởi tạo bảng:
    ```bash
    # Sử dụng công cụ migrate (ví dụ: golang-migrate)
    migrate -path migrations -database "postgres://user:pass@localhost:5432/dbname?sslmode=disable" up
    ```

## Chạy ứng dụng

Để chạy server:

```bash
go run cmd/server/main.go
```
