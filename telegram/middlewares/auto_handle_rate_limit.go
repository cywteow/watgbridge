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
		log.Printf("[auto_handle_rate_limit] waiting %.1fs for rate-limit backoff to expire (until %s)",
			remaining.Seconds(), until.Format(time.RFC3339))
		time.Sleep(remaining)
		log.Printf("[auto_handle_rate_limit] rate-limit backoff expired, resuming")
	}
}

// setTelegramRateLimit extends the backoff deadline if the new deadline is later.
func setTelegramRateLimit(d time.Duration) {
	tgRateLimitMu.Lock()
	defer tgRateLimitMu.Unlock()
	candidate := time.Now().Add(d)
	if candidate.After(tgRateLimitUntil) {
		prev := tgRateLimitUntil
		tgRateLimitUntil = candidate
		if prev.IsZero() || time.Until(prev) <= 0 {
			log.Printf("[auto_handle_rate_limit] rate-limit set: backoff for %v (until %s)",
				d.Round(time.Second), candidate.Format(time.RFC3339))
		} else {
			log.Printf("[auto_handle_rate_limit] rate-limit extended by %v (new deadline: %s)",
				d.Round(time.Second), candidate.Format(time.RFC3339))
		}
	} else {
		log.Printf("[auto_handle_rate_limit] rate-limit NOT extended (existing deadline %s is later)",
			tgRateLimitUntil.Format(time.RFC3339))
	}
}

type autoHandleRateLimitBotClient struct {
	gotgbot.BotClient
}

func (b *autoHandleRateLimitBotClient) RequestWithContext(ctx context.Context,
	token string, method string, params map[string]string,
	data map[string]gotgbot.FileReader,
	opts *gotgbot.RequestOpts) (json.RawMessage, error) {

	attempt := 0
	for {
		// Respect the global rate-limit backoff before attempting the request.
		WaitTelegramRateLimit()

		attempt++
		if attempt > 1 {
			log.Printf("[auto_handle_rate_limit] retrying %s (attempt %d)", method, attempt)
		}

		response, err := b.BotClient.RequestWithContext(ctx, token, method, params, data, opts)
		if err == nil {
			if attempt > 1 {
				log.Printf("[auto_handle_rate_limit] %s succeeded after %d attempts", method, attempt)
			}
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
			log.Printf("[auto_handle_rate_limit] 429 on %s – backing off %ds (attempt %d)", method, timeToSleep, attempt)
			setTelegramRateLimit(d)
			continue
		}

		return response, err
	}
}

func AutoHandleRateLimit(b gotgbot.BotClient) gotgbot.BotClient {
	return &autoHandleRateLimitBotClient{b}
}
