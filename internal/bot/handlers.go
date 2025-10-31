package bot

import (
	"context"
	"fbuidwatcher/internal/checker"
	"fbuidwatcher/internal/model"
	"fbuidwatcher/internal/storage"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const minIntervalSec = 300 // 5 phút

type Handlers struct {
	api         *tgbotapi.BotAPI
	store       *storage.FileStore
	fb          *checker.FBChecker
	cancelMu    sync.Mutex
	cancelTasks map[string]context.CancelFunc // key = ownerID:uid
}

func NewHandlers(api *tgbotapi.BotAPI, store *storage.FileStore) *Handlers {
	return &Handlers{
		api:         api,
		store:       store,
		fb:          checker.NewFBChecker(),
		cancelTasks: make(map[string]context.CancelFunc),
	}
}

func (h *Handlers) Handle(upd tgbotapi.Update) {
	msg := strings.TrimSpace(upd.Message.Text)
	chatID := upd.Message.Chat.ID
	o := fmt.Sprintf("%d", chatID)

	switch {
	case strings.HasPrefix(msg, "/start"), strings.HasPrefix(msg, "/help"):
		h.reply(chatID, `Xin chào! Bot theo dõi live/die UID Facebook.

Lệnh:
- /add <uid> [ghi chú]         → dùng delay chung hiện tại
- /list
- /stats
- /setdelay <interval>         → ví dụ: /setdelay 10m (tối thiểu 5m)
- /getdelay
- /remove <uid>
- /clear`)

	case strings.HasPrefix(msg, "/add"):
		parts := strings.Fields(msg)
		if len(parts) < 2 {
			h.reply(chatID, "Sai cú pháp. Ví dụ: /add 1000123456789 acc phụ")
			return
		}
		uid := parts[1]
		if _, err := strconv.ParseInt(uid, 10, 64); err != nil {
			h.reply(chatID, "UID phải là số.")
			return
		}
		note := ""
		if len(parts) > 2 {
			note = strings.TrimSpace(strings.Join(parts[2:], " "))
		}

		// load
		ds, _ := h.store.Load()
		od := ds[o]
		if od.Items == nil {
			od.Items = map[string]model.WatchInfo{}
		}
		if od.DefaultIntervalSec <= 0 {
			od.DefaultIntervalSec = minIntervalSec
		}

		wi := od.Items[uid]
		wi.UID = uid
		wi.Note = note
		wi.AddedAtUnix = time.Now().Unix()
		// đồng bộ interval của UID = default hiện tại
		wi.IntervalSec = od.DefaultIntervalSec
		od.Items[uid] = wi
		ds[o] = od
		_ = h.store.Save(ds)

		h.startWatch(chatID, uid, wi.IntervalSec)
		msg := fmt.Sprintf("✅ Theo dõi UID `%s` mỗi %d giây (gửi tin sau mỗi lần re-check).", uid, wi.IntervalSec)
		if note != "" {
			msg += fmt.Sprintf("\n📝 _%s_", escapeMD(note))
		}
		h.replyMD(chatID, msg)

	case strings.HasPrefix(msg, "/setdelay"):
		parts := strings.Fields(msg)
		if len(parts) < 2 {
			h.reply(chatID, "Sai cú pháp. Ví dụ: /setdelay 10m (tối thiểu 5m)")
			return
		}
		sec, err := parseIntervalToSeconds(parts[1])
		if err != nil || sec < 1 {
			h.reply(chatID, "Khoảng thời gian không hợp lệ. Dùng 5m, 10m, 1h, 1d.")
			return
		}
		if sec < minIntervalSec {
			sec = minIntervalSec
		}

		// cập nhật default + restart tất cả UID của user này
		ds, _ := h.store.Load()
		od := ds[o]
		if od.Items == nil {
			od.Items = map[string]model.WatchInfo{}
		}
		od.DefaultIntervalSec = sec
		// cập nhật interval từng UID
		for uid, wi := range od.Items {
			wi.IntervalSec = sec
			od.Items[uid] = wi
		}
		ds[o] = od
		_ = h.store.Save(ds)

		// restart watches
		for uid := range od.Items {
			h.startWatch(chatID, uid, sec)
		}

		h.reply(chatID, fmt.Sprintf("⏱️ Đã đặt delay chung = %d giây cho tất cả UID.", sec))

	case strings.HasPrefix(msg, "/getdelay"):
		ds, _ := h.store.Load()
		od := ds[o]
		if od.DefaultIntervalSec <= 0 {
			od.DefaultIntervalSec = minIntervalSec
		}
		h.reply(chatID, fmt.Sprintf("⏱️ Delay hiện tại: %d giây.", od.DefaultIntervalSec))

	case strings.HasPrefix(msg, "/list"):
		h.reply(chatID, h.listWatches(chatID))

	case strings.HasPrefix(msg, "/stats"):
		h.reply(chatID, h.stats(chatID))

	case strings.HasPrefix(msg, "/remove"), strings.HasPrefix(msg, "/stop"):
		parts := strings.Fields(msg)
		if len(parts) < 2 {
			h.reply(chatID, "Sai cú pháp. Ví dụ: /remove 1000123456789")
			return
		}
		h.stopWatch(chatID, parts[1])
		h.replyMD(chatID, fmt.Sprintf("🗑️ Đã bỏ theo dõi UID `%s`.", parts[1]))

	case strings.HasPrefix(msg, "/clear"):
		h.clearAll(chatID)
		h.reply(chatID, "🧹 Đã dừng theo dõi tất cả UID của bạn.")
	}
}

// ---------- Core watch ----------
func (h *Handlers) startWatch(ownerID int64, uid string, intervalSec int) {
	if intervalSec < minIntervalSec {
		intervalSec = minIntervalSec
	}

	k := fmt.Sprintf("%d:%s", ownerID, uid)

	// cancel nếu đã tồn tại
	h.cancelMu.Lock()
	if cancel, ok := h.cancelTasks[k]; ok {
		cancel()
		delete(h.cancelTasks, k)
	}
	h.cancelMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	h.cancelMu.Lock()
	h.cancelTasks[k] = cancel
	h.cancelMu.Unlock()

	go func() {
		// Check lần đầu
		st := h.fb.CheckLive(uid)
		isLive := (st == "live")
		h.replyMD(ownerID, fmt.Sprintf("Initial check `%s` → %s", uid, humanStatus(st)))
		h.updateLastStatus(ownerID, uid, isLive, intervalSec)

		ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				h.replyMD(ownerID, fmt.Sprintf("🛑 Đã dừng theo dõi UID `%s`.", uid))
				return
			case <-ticker.C:
				st := h.fb.CheckLive(uid)
				isLive := (st == "live")

				// ✅ Luôn gửi tin mỗi lần re-check
				h.replyMD(ownerID, fmt.Sprintf("Re-check `%s` @ %s → %s",
					uid, time.Now().Format("15:04:05"), humanStatus(st)))

				h.updateLastStatus(ownerID, uid, isLive, intervalSec)
			}
		}
	}()
}

// ---------- Views / helpers ----------
func (h *Handlers) listWatches(ownerID int64) string {
	ds, _ := h.store.Load()
	o := fmt.Sprintf("%d", ownerID)
	od := ds[o]
	if len(od.Items) == 0 {
		return "Bạn chưa theo dõi UID nào."
	}

	// sắp xếp theo thời gian thêm
	type row struct {
		uid  string
		info model.WatchInfo
	}
	rows := make([]row, 0, len(od.Items))
	for uid, info := range od.Items {
		rows = append(rows, row{uid: uid, info: info})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].info.AddedAtUnix < rows[j].info.AddedAtUnix })

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Delay chung: %d giây\n", od.DefaultIntervalSec))
	b.WriteString("Danh sách UID bạn đang theo dõi:\n")
	for _, r := range rows {
		status := "—"
		if r.info.LastStatus != nil {
			if *r.info.LastStatus {
				status = "✅ LIVE"
			} else {
				status = "❌ DIE"
			}
		}
		if r.info.Note != "" {
			b.WriteString(fmt.Sprintf("- `%s` | %ds | last: %s | 📝 %s\n",
				r.uid, r.info.IntervalSec, status, r.info.Note))
		} else {
			b.WriteString(fmt.Sprintf("- `%s` | %ds | last: %s\n",
				r.uid, r.info.IntervalSec, status))
		}
	}
	return b.String()
}

func (h *Handlers) stats(ownerID int64) string {
	ds, _ := h.store.Load()
	o := fmt.Sprintf("%d", ownerID)
	od := ds[o]
	if len(od.Items) == 0 {
		return "Chưa có dữ liệu thống kê (bạn chưa theo dõi UID nào)."
	}
	total, live, die, unknown := 0, 0, 0, 0
	for _, info := range od.Items {
		total++
		if info.LastStatus == nil {
			unknown++
			continue
		}
		if *info.LastStatus {
			live++
		} else {
			die++
		}
	}
	return fmt.Sprintf("📊 Thống kê của bạn:\n- Tổng UID: %d\n- ✅ LIVE: %d\n- ❌ DIE: %d\n- Chưa biết: %d",
		total, live, die, unknown)
}

func (h *Handlers) stopWatch(ownerID int64, uid string) {
	// cancel goroutine
	k := fmt.Sprintf("%d:%s", ownerID, uid)
	h.cancelMu.Lock()
	if cancel, ok := h.cancelTasks[k]; ok {
		cancel()
		delete(h.cancelTasks, k)
	}
	h.cancelMu.Unlock()

	// xóa khỏi storage
	ds, _ := h.store.Load()
	o := fmt.Sprintf("%d", ownerID)
	od := ds[o]
	if od.Items != nil {
		delete(od.Items, uid)
	}
	ds[o] = od
	_ = h.store.Save(ds)
}

func (h *Handlers) clearAll(ownerID int64) {
	// cancel tất cả uid của owner
	prefix := fmt.Sprintf("%d:", ownerID)
	h.cancelMu.Lock()
	for kk, cancel := range h.cancelTasks {
		if strings.HasPrefix(kk, prefix) {
			cancel()
			delete(h.cancelTasks, kk)
		}
	}
	h.cancelMu.Unlock()

	// xóa dữ liệu owner
	ds, _ := h.store.Load()
	delete(ds, fmt.Sprintf("%d", ownerID))
	_ = h.store.Save(ds)
}

func (h *Handlers) updateLastStatus(ownerID int64, uid string, isLive bool, interval int) {
	ds, _ := h.store.Load()
	o := fmt.Sprintf("%d", ownerID)
	od := ds[o]
	if od.Items == nil {
		od.Items = map[string]model.WatchInfo{}
	}
	wi := od.Items[uid]
	wi.UID = uid
	wi.IntervalSec = interval
	wi.LastStatus = &isLive
	if wi.AddedAtUnix == 0 {
		wi.AddedAtUnix = time.Now().Unix()
	}
	od.Items[uid] = wi
	if od.DefaultIntervalSec < minIntervalSec {
		od.DefaultIntervalSec = minIntervalSec
	}
	ds[o] = od
	_ = h.store.Save(ds)
}

// ---------- small I/O helpers ----------
func (h *Handlers) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	h.api.Send(msg)
}
func (h *Handlers) replyMD(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	h.api.Send(msg)
}
func humanStatus(st string) string {
	switch st {
	case "live":
		return "✅ LIVE"
	case "die":
		return "❌ DIE"
	default:
		return "⚠️ ERROR"
	}
}
func escapeMD(s string) string {
	repl := strings.NewReplacer("_", "\\_", "*", "\\*", "`", "\\`", "[", "\\[")
	return repl.Replace(s)
}

// ---------- Khôi phục khi bot khởi động ----------
func (h *Handlers) RestoreWatches() {
	ds, _ := h.store.Load()
	for owner, od := range ds {
		oid, err := strconv.ParseInt(owner, 10, 64)
		if err != nil {
			continue
		}
		interval := od.DefaultIntervalSec
		if interval < minIntervalSec {
			interval = minIntervalSec
		}
		for uid := range od.Items {
			h.startWatch(oid, uid, interval)
		}
	}
}

// parseIntervalToSeconds: local (tránh phụ thuộc utils nếu bạn đã bỏ)
func parseIntervalToSeconds(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty interval")
	}
	unit := s[len(s)-1]
	num := s
	mult := 1
	switch unit {
	case 's':
		num = s[:len(s)-1]
		mult = 1
	case 'm':
		num = s[:len(s)-1]
		mult = 60
	case 'h':
		num = s[:len(s)-1]
		mult = 3600
	case 'd':
		num = s[:len(s)-1]
		mult = 86400
	default:
		num = s
		mult = 1
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid interval")
	}
	return int(v * float64(mult)), nil
}
