package middlewares

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

var (
	tgRateLimitMu    sync.Mutex
	tgRateLimitUntil time.Time
)

// WaitTelegramRateLimit blocks until the global Telegram rate-limit backoff has expired.
// Safe to call from multiple goroutines concurrently.
func WaitTelegramRateLimit() {
	for {
		tgRateLimitMu.Lock()
		until := tgRateLimitUntil
		tgRateLimitMu.Unlock()

		remaining := time.Until(until)
		if remaining <= 0 {
			return
		}
		time.Sleep(remaining)
	}
}

// setTelegramRateLimit extends the backoff deadline if the new deadline is later.
func setTelegramRateLimit(d time.Duration) {
	tgRateLimitMu.Lock()
	defer tgRateLimitMu.Unlock()
	candidate := time.Now().Add(d)
	if candidate.After(tgRateLimitUntil) {
		tgRateLimitUntil = candidate
	}
}

type autoHandleRateLimitBotClient struct {
	gotgbot.BotClient
}

func (b *autoHandleRateLimitBotClient) RequestWithContext(ctx context.Context,
	token string, method string, params map[string]string,
	data map[string]gotgbot.FileReader,
	opts *gotgbot.RequestOpts) (json.RawMessage, error) {

	for {
		// Respect the global rate-limit backoff before attempting the request.
		WaitTelegramRateLimit()

		response, err := b.BotClient.RequestWithContext(ctx, token, method, params, data, opts)
		if err == nil {
			return response, err
		}

		tgError, ok := err.(*gotgbot.TelegramError)
		if !ok {
			return response, err
		}

		if tgError.Code == 429 {
			fields := strings.Fields(tgError.Description)
			timeToSleep, _ := strconv.ParseInt(fields[len(fields)-1], 10, 64)
			d := time.Duration(timeToSleep) * time.Second
			log.Printf("[auto_handle_rate_limit] rate limited, backing off for %v seconds", timeToSleep)
			setTelegramRateLimit(d)
			continue
		}

		return response, err
	}
}

func AutoHandleRateLimit(b gotgbot.BotClient) gotgbot.BotClient {
	return &autoHandleRateLimitBotClient{b}
}
