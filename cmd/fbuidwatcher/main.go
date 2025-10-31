package main

import (
	"fbuidwatcher/internal/bot"
	"fbuidwatcher/internal/config"
	"fbuidwatcher/internal/storage"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	// Load config (.env)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// Init Telegram Bot API
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("telegram init error: %v", err)
	}
	api.Debug = false
	log.Printf("Authorized on %s", api.Self.UserName)

	// Init storage
	store := storage.NewFileStore("data.json")

	// Handlers
	h := bot.NewHandlers(api, store)

	// Restore previously watched UIDs
	h.RestoreWatches()

	// Listen for updates
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		go h.Handle(update)
	}
}
