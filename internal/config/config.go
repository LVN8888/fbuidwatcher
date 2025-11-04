package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

type Config struct {
	TelegramToken string
}

func Load() (*Config, error) {
	file, err := os.Open(".env")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				os.Setenv(key, val)
			}
		}
	}

	token := os.Getenv("TG_BOT_TOKEN")
	if token == "" {
		return nil, errors.New("TG_BOT_TOKEN is required in .env")
	}

	return &Config{TelegramToken: token}, nil
}
