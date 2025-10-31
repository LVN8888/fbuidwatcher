package bot

import (
	"context"
	"fbuidwatcher/internal/checker"
	"fbuidwatcher/internal/model"
	"fbuidwatcher/internal/storage"
	"fbuidwatcher/internal/utils"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Handlers struct {
	api         *tgbotapi.BotAPI
	store       *storage.FileStore
	fb          *checker.FBChecker
	cancelMu    sync.Mutex
	cancelTasks map[string]context.CancelFunc // key: ownerID:uid
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

	switch {
	case strings.HasPrefix(msg, "/start"), strings.HasPrefix(msg, "/help"):
		h.reply(chatID, `Xin chào! Bot check live/die UID Facebook (no token).

Lệnh:
- /add <uid> <interval> [ghi chú]
  ví dụ: /add 1000123456789 30s acc phụ
- /list
- /stats
- /remove <uid>  (hoặc /stop <uid>)
- /clear`)

	case strings.HasPrefix(msg, "/add"):
		parts := strings.Fields(msg)
		if len(parts) < 3 {
			h.reply(chatID, "Sai cú pháp. Ví dụ: /add 1000123456789 30s ghi chú")
			return
		}
		uid := parts[1]
		if _, err := strconv.ParseInt(uid, 10, 64); err != nil {
			h.reply(chatID, "UID phải là số.")
			return
		}
		sec, err := utils.ParseIntervalToSeconds(parts[2])
		if err != nil || sec < 1 {
			h.reply(chatID, "Khoảng thời gian không hợp lệ. Dùng 30s, 10m, 1h, 1d.")
			return
		}
		note := utils.QuoteJoin(parts, 3)

		ds, _ := h.store.Load()
		o := fmt.Sprintf("%d", chatID)
		if _, ok := ds[o]; !ok {
			ds[o] = map[string]model.WatchInfo{}
		}
		info := ds[o][uid]
		info.UID = uid
		info.IntervalSec = sec
		info.Note = note
		info.AddedAtUnix = time.Now().Unix()
		ds[o][uid] = info
		_ = h.store.Save(ds)

		h.startWatch(chatID, uid, sec)
		if note != "" {
			h.replyMD(chatID, fmt.Sprintf("✅ Theo dõi UID `%s` mỗi %d giây.\n📝 _%s_", uid, sec, escapeMD(note)))
		} else {
			h.replyMD(chatID, fmt.Sprintf("✅ Theo dõi UID `%s` mỗi %d giây.", uid, sec))
		}

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
		// check lần đầu
		status := h.fb.CheckLive(uid)
		h.replyMD(ownerID, fmt.Sprintf("Initial check for UID `%s`: %s", uid, humanStatus(status)))
		h.updateLastStatus(ownerID, uid, status == "live", intervalSec)

		ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer ticker.Stop()

		var prev *bool
		s := (status == "live")
		prev = &s

		for {
			select {
			case <-ctx.Done():
				h.replyMD(ownerID, fmt.Sprintf("🛑 Đã dừng theo dõi UID `%s`.", uid))
				return
			case <-ticker.C:
				st := h.fb.CheckLive(uid)
				isLive := (st == "live")
				if prev == nil || *prev != isLive {
					h.replyMD(ownerID, fmt.Sprintf("Status change for UID `%s`: %s", uid, humanStatus(st)))
				}
				prev = &isLive
				h.updateLastStatus(ownerID, uid, isLive, intervalSec)
			}
		}
	}()
}

// ---------- Views / helpers ----------
func (h *Handlers) listWatches(ownerID int64) string {
	ds, _ := h.store.Load()
	o := fmt.Sprintf("%d", ownerID)
	w := ds[o]
	if len(w) == 0 {
		return "Bạn chưa theo dõi UID nào."
	}

	// sắp xếp theo thời gian thêm
	type row struct {
		uid  string
		info model.WatchInfo
	}
	rows := make([]row, 0, len(w))
	for uid, info := range w {
		rows = append(rows, row{uid: uid, info: info})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].info.AddedAtUnix < rows[j].info.AddedAtUnix })

	var b strings.Builder
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
	w := ds[o]
	if len(w) == 0 {
		return "Chưa có dữ liệu thống kê (bạn chưa theo dõi UID nào)."
	}
	total, live, die, unknown := 0, 0, 0, 0
	for _, info := range w {
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
	if m, ok := ds[o]; ok {
		delete(m, uid)
		if len(m) == 0 {
			delete(ds, o)
		} else {
			ds[o] = m
		}
	}
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
	if _, ok := ds[o]; !ok {
		ds[o] = map[string]model.WatchInfo{}
	}
	wi := ds[o][uid]
	wi.UID = uid
	wi.IntervalSec = interval
	wi.LastStatus = &isLive
	if wi.AddedAtUnix == 0 {
		wi.AddedAtUnix = time.Now().Unix()
	}
	ds[o][uid] = wi
	_ = h.store.Save(ds)
}

// small I/O helpers
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

// Khôi phục các UID đang theo dõi khi bot khởi động lại
func (h *Handlers) RestoreWatches() {
	ds, _ := h.store.Load()
	for owner, m := range ds {
		oid, err := strconv.ParseInt(owner, 10, 64)
		if err != nil {
			continue
		}
		for uid, info := range m {
			interval := info.IntervalSec
			if interval <= 0 {
				interval = 60
			}
			h.startWatch(oid, uid, interval)
		}
	}
}
