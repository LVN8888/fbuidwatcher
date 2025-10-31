package checker

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type FBChecker struct {
	Client *http.Client
}

func NewFBChecker() *FBChecker {
	return &FBChecker{
		Client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: false,
			},
		},
	}
}

func (c *FBChecker) CheckLive(uid string) string {
	url := fmt.Sprintf("https://graph.facebook.com/%s/picture?redirect=false", uid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.131 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "error"
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if strings.Contains(s, "height") && strings.Contains(s, "width") {
		return "live"
	}
	return "die"
}
