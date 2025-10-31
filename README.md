# 🚀 FB UID Watcher — Telegram Bot

Bot Telegram giúp **theo dõi trạng thái LIVE/DIE của UID Facebook** tự động, gửi thông báo mỗi lần re-check và lưu dữ liệu cục bộ.  
> Liên hệ hỗ trợ qua telegram: **@hetcuuae**

---

## 🧩 Giới thiệu

**FB UID Watcher** là một bot viết bằng **Golang**, giúp bạn theo dõi trạng thái **UID Facebook** theo thời gian thực.  
Bot sẽ kiểm tra định kỳ (ví dụ 10 phút/lần) và gửi thông báo mỗi khi UID vẫn LIVE hoặc đã DIE.

---

## ⚙️ Tính năng nổi bật

✅ Theo dõi nhiều UID cùng lúc  
✅ Đặt **delay** riêng cho từng UID (5s, 10m, 1h, …)  
✅ Ghi chú riêng cho từng UID  
✅ Gửi thông báo mỗi lần kiểm tra  
✅ Lưu file cục bộ (`data.json`) — không cần database  
✅ Khôi phục tự động sau khi bot khởi động lại  

---

## 🧭 Cấu trúc dự án

fbuidwatcher/
├── cmd/
│ └── fbuidwatcher/
│ └── main.go # Điểm khởi chạy chính
├── internal/
│ ├── bot/ # Xử lý Telegram Bot & Handler
│ ├── checker/ # Logic kiểm tra UID Facebook
│ ├── config/ # Đọc file .env
│ ├── model/ # Struct dữ liệu
│ └── storage/ # Lưu & đọc data.json
├── .env.example
├── README.md
├── .gitignore
├── go.mod
└── go.sum


---

## 🧠 Yêu cầu

- Go >= **1.21**
- Telegram Bot Token từ [@BotFather](https://t.me/BotFather)
