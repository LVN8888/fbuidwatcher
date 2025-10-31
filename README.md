# 🚀 FB UID Watcher — Telegram Bot

**FB UID Watcher** là một bot Telegram viết bằng **Golang**, giúp bạn theo dõi trạng thái **LIVE/DIE của UID Facebook** một cách tự động và liên tục.  
Mỗi UID được kiểm tra định kỳ và bot sẽ gửi thông báo đến Telegram mỗi khi UID vẫn còn sống hoặc đã die.

> 💬 Liên hệ hỗ trợ: [@hetcuuae](https://t.me/hetcuuae)  
> 👨‍💻 Phát triển bởi: **@lvnsoftware**

---

## ✨ Tính năng chính

- ✅ Theo dõi nhiều UID Facebook cùng lúc
- ⏱️ Cài đặt thời gian kiểm tra riêng cho từng UID (`5s`, `10m`, `1h`, v.v)
- 📌 Ghi chú cho từng UID
- 🔔 Gửi thông báo Telegram mỗi lần re-check
- 💾 Lưu trữ UID cục bộ bằng file `data.json` (không cần database)
- 🔄 Tự động khôi phục trạng thái khi khởi động lại bot

## 🧱 Cấu trúc dự án

```bash
fbuidwatcher/
├── cmd/
│ └── fbuidwatcher/
│ └── main.go # Điểm khởi chạy chính
├── internal/
│ ├── bot/ # Xử lý Telegram Bot & Handler
│ ├── checker/ # Logic kiểm tra UID Facebook
│ ├── config/ # Đọc biến môi trường từ .env
│ ├── model/ # Định nghĩa struct dữ liệu
│ └── storage/ # Ghi/đọc file data.json
├── .env.example # File cấu hình mẫu
├── .env # File cấu hình chuẩn
├── .gitignore
├── README.md
├── go.mod
└── go.sum
```

## ⚙️ Hướng dẫn cài đặt nhanh

### 1. Clone project

```bash
git clone https://github.com/LVN8888/fbuidwatcher.git
cd fbuidwatcher
```

### 2. Cài đặt môi trường

```bash
cp .env.example .env
go mod tidy
```
### 3. Điền token vào .env
```bash
TELEGRAM_TOKEN=YOUR_BOT_TOKEN_HERE
```
### 4. Chạy bot
```bash
go run ./cmd/fbuidwatcher
```

## 🏗️ Build ra file .exe (nếu cần cho Windows)
```bash
GOOS=windows GOARCH=amd64 go build -o fbuidwatcher.exe ./cmd/fbuidwatcher
```
Sau khi build thành công, bạn sẽ có file fbuidwatcher.exe để chạy trên máy Windows.

## 📜 Giấy phép sử dụng

- Dự án sử dụng giấy phép MIT License — miễn phí sử dụng, chia sẻ và chỉnh sửa.
- ⭐ Nếu dự án này hữu ích, hãy để lại một ⭐ Star trên GitHub nhé!