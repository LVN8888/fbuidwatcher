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

// NOTE: để test cho phép 5 giây; khi chạy thật đổi thành 300 (5 phút)
const minIntervalSec = 5

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
	if upd.CallbackQuery != nil {
		h.handleCallback(upd.CallbackQuery)
		return
	}
	if upd.Message == nil {
		return
	}

	msg := strings.TrimSpace(upd.Message.Text)
	chatID := upd.Message.Chat.ID
	o := fmt.Sprintf("%d", chatID)

	switch {
	case strings.HasPrefix(msg, "/start"), strings.HasPrefix(msg, "/help"):
		h.replyWithMainMenu(chatID)

	case strings.HasPrefix(msg, "/add"):
		parts := strings.Fields(msg)
		if len(parts) < 3 {
			h.reply(chatID, "❌ Sai cú pháp.\nCú pháp đúng: `/add <uid> <delay> [ghi_chú]`\nVí dụ: `/add 100004253947596 10m Kèo unlock`")
			return
		}
		uid := parts[1]
		if _, err := strconv.ParseInt(uid, 10, 64); err != nil {
			h.reply(chatID, "⚠️ UID phải là số.")
			return
		}
		sec, err := parseIntervalToSeconds(parts[2])
		if err != nil || sec < 1 {
			h.reply(chatID, "⚠️ Delay không hợp lệ. Dùng 5s, 10m, 1h, ...")
			return
		}
		if sec < minIntervalSec {
			sec = minIntervalSec
		}
		note := ""
		if len(parts) > 3 {
			note = strings.TrimSpace(strings.Join(parts[3:], " "))
		}

		ds, _ := h.store.Load()
		od := ds[o]
		if od.Items == nil {
			od.Items = map[string]model.WatchInfo{}
		}
		wi := od.Items[uid]
		wi.UID = uid
		wi.Note = note
		wi.AddedAtUnix = time.Now().Unix()
		wi.IntervalSec = sec
		od.Items[uid] = wi
		ds[o] = od
		_ = h.store.Save(ds)

		h.startWatch(chatID, uid, wi.IntervalSec)

		var text strings.Builder
		text.WriteString(fmt.Sprintf("✅ **Đã theo dõi UID `%s` mỗi %d giây.**", uid, wi.IntervalSec))
		if note != "" {
			text.WriteString(fmt.Sprintf("\n📝 *Ghi chú:* %s", note))
		}
		h.replyWithUIDMenu(chatID, text.String())

	case strings.HasPrefix(msg, "/list"):
		h.reply(chatID, h.listWatches(chatID))

	case strings.HasPrefix(msg, "/stats"):
		h.reply(chatID, h.stats(chatID))

	case strings.HasPrefix(msg, "/remove"), strings.HasPrefix(msg, "/stop"):
		parts := strings.Fields(msg)
		if len(parts) < 2 {
			h.reply(chatID, "❌ Sai cú pháp.\nVí dụ: `/remove 1000123456789`")
			return
		}
		h.stopWatch(chatID, parts[1])
		h.reply(chatID, fmt.Sprintf("🗑️ Đã dừng theo dõi UID `%s`.", parts[1]))

	case strings.HasPrefix(msg, "/clear"):
		h.clearAll(chatID)
		h.reply(chatID, "🧹 Đã dừng theo dõi tất cả UID của bạn.")
	}
}

// ---------- Callback (inline keyboard) ----------
func (h *Handlers) handleCallback(cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	chatID := cb.Message.Chat.ID

	switch {
	case strings.HasPrefix(data, "stop:"):
		uid := strings.TrimPrefix(data, "stop:")
		h.stopWatch(chatID, uid)
		h.answerCB(cb, "Đã dừng "+uid)
		edit := tgbotapi.NewEditMessageReplyMarkup(chatID, cb.Message.MessageID, tgbotapi.InlineKeyboardMarkup{})
		h.api.Send(edit)

	case data == "list":
		h.answerCB(cb, "📋 Danh sách UID")
		h.reply(chatID, h.listWatches(chatID))
	}
}

func (h *Handlers) answerCB(cb *tgbotapi.CallbackQuery, text string) {
	_, _ = h.api.Request(tgbotapi.NewCallback(cb.ID, text))
}

// ---------- Core watch ----------
func (h *Handlers) startWatch(ownerID int64, uid string, intervalSec int) {
	if intervalSec < minIntervalSec {
		intervalSec = minIntervalSec
	}

	k := fmt.Sprintf("%d:%s", ownerID, uid)
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
		st := h.fb.CheckLive(uid)
		isLive := (st == "live")
		h.sendUIDStatus(ownerID, uid, st, intervalSec, true)
		h.updateLastStatus(ownerID, uid, isLive, intervalSec)

		ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				h.reply(ownerID, fmt.Sprintf("🛑 Đã dừng theo dõi UID %s.", uid))
				return
			case <-ticker.C:
				st := h.fb.CheckLive(uid)
				isLive := (st == "live")
				h.sendUIDStatus(ownerID, uid, st, intervalSec, false)
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
		return "⚠️ Bạn chưa theo dõi UID nào."
	}

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
	b.WriteString("📋 **Danh sách UID bạn đang theo dõi:**\n\n")
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
			b.WriteString(fmt.Sprintf("• `%s` | %ds | %s | 📝 %s\n",
				r.uid, r.info.IntervalSec, status, r.info.Note))
		} else {
			b.WriteString(fmt.Sprintf("• `%s` | %ds | %s\n",
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
		return "⚠️ Chưa có dữ liệu thống kê (bạn chưa theo dõi UID nào)."
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
	return fmt.Sprintf("📊 **Thống kê của bạn:**\n- Tổng UID: %d\n- ✅ LIVE: %d\n- ❌ DIE: %d\n- ❔ Chưa biết: %d",
		total, live, die, unknown)
}

func (h *Handlers) stopWatch(ownerID int64, uid string) {
	k := fmt.Sprintf("%d:%s", ownerID, uid)
	h.cancelMu.Lock()
	if cancel, ok := h.cancelTasks[k]; ok {
		cancel()
		delete(h.cancelTasks, k)
	}
	h.cancelMu.Unlock()

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
	prefix := fmt.Sprintf("%d:", ownerID)
	h.cancelMu.Lock()
	for kk, cancel := range h.cancelTasks {
		if strings.HasPrefix(kk, prefix) {
			cancel()
			delete(h.cancelTasks, kk)
		}
	}
	h.cancelMu.Unlock()

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
	ds[o] = od
	_ = h.store.Save(ds)
}

// ---------- UI helpers ----------
func (h *Handlers) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	h.api.Send(msg)
}

func (h *Handlers) replyWithMainMenu(chatID int64) {
	intro := `👋 *Chào mừng đến với Bot theo dõi UID Facebook!*
💡 Công cụ miễn phí được phát triển bởi **@lvnsoftware** giúp bạn kiểm tra trạng thái LIVE/DIE của UID Facebook tự động 1 cách nhanh chóng.

*⚙️ HƯỚNG DẪN SỬ DỤNG:*

📌 */add <uid> <delay> [ghi_chú]* → Bắt đầu theo dõi 1 UID  
📋 */list* → Danh sách đang theo dõi  
📊 */stats* → Thống kê LIVE/DIE  
🗑 */remove <uid>* → Dừng & xoá 1 UID  
🚫 */clear* → Dừng & xoá tất cả UID  

📩 Liên hệ hỗ trợ: *@hetcuuae*
`

	msg := tgbotapi.NewMessage(chatID, intro)
	msg.ParseMode = "Markdown"
	h.api.Send(msg)
}

func (h *Handlers) replyWithUIDMenu(chatID int64, text string) {
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

func (h *Handlers) sendUIDStatus(chatID int64, uid, st string, interval int, first bool) {
	prefix := "Re-check"
	if first {
		prefix = "Initial"
	}
	line := fmt.Sprintf("UID: `%s` @ %s → %s", uid, time.Now().Format("15:04:05"), humanStatus(st))
	if first {
		line = fmt.Sprintf("%s | delay %ds\n%s", prefix, interval, line)
	}
	msg := tgbotapi.NewMessage(chatID, line)
	msg.ParseMode = "Markdown"
	h.api.Send(msg)
}

// ---------- Khôi phục khi bot khởi động ----------
func (h *Handlers) RestoreWatches() {
	ds, _ := h.store.Load()
	for owner, od := range ds {
		oid, err := strconv.ParseInt(owner, 10, 64)
		if err != nil {
			continue
		}
		for uid, wi := range od.Items {
			iv := wi.IntervalSec
			if iv < minIntervalSec {
				iv = minIntervalSec
			}
			h.startWatch(oid, uid, iv)
		}
	}
}

// parseIntervalToSeconds: "5s", "10m", "1h", "2d"...
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
