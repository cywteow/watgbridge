// Package queue provides rate-limited send queues for WhatsApp and Telegram
// to prevent flooding their respective API servers.
// All outbound sends should go through WaSend / TgRun (or the typed Tg* wrappers).
package queue

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"watgbridge/state"
	"watgbridge/telegram/middlewares"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
)

const QueueSize = 1000

// NOTE: Do NOT initialize WaInterval / TgInterval as package-level vars from
// state.State.Config here – config is not yet loaded at package-init time, so
// those values would always be 0. Workers read the config on every iteration
// instead (see waWorker / tgWorker).

var waJobCh = make(chan func(), QueueSize)
var tgJobCh = make(chan func(), QueueSize)

// counters for log correlation
var waJobCounter atomic.Int64
var tgJobCounter atomic.Int64

// StartWorkers launches the background rate-limited sender goroutines.
// Must be called exactly once at startup, AFTER the config has been loaded.
func StartWorkers() {
	log.Printf("[queue] starting workers (queue size: %d)", QueueSize)
	go waWorker()
	go tgWorker()
	log.Printf("[queue] workers started")
}

func waWorker() {
	log.Printf("[wa_queue] worker ready")
	for job := range waJobCh {
		seq := waJobCounter.Add(1)
		depth := len(waJobCh)
		log.Printf("[wa_queue] job #%d started (remaining in queue: %d)", seq, depth)
		job()
		log.Printf("[wa_queue] job #%d completed", seq)

		if state.State.Config.WhatsApp.QueueEnabled {
			// Read interval from config on every tick so config changes take effect.
			interval := time.Duration(state.State.Config.WhatsApp.QueueIntervalMs) * time.Millisecond
			if interval > 0 {
				log.Printf("[wa_queue] throttling %v before next job", interval)
				time.Sleep(interval)
			}
		}
	}
}

func tgWorker() {
	log.Printf("[tg_queue] worker ready")
	for job := range tgJobCh {
		seq := tgJobCounter.Add(1)
		depth := len(tgJobCh)
		log.Printf("[tg_queue] job #%d dequeued (remaining in queue: %d)", seq, depth)

		// Wait out any active rate-limit backoff BEFORE dispatching so we never
		// fire a request we already know will be rejected.
		middlewares.WaitTelegramRateLimit()

		log.Printf("[tg_queue] job #%d dispatching", seq)
		job()
		log.Printf("[tg_queue] job #%d completed", seq)

		if state.State.Config.Telegram.QueueEnabled {
			// Read interval from config on every tick so config changes take effect.
			interval := time.Duration(state.State.Config.Telegram.QueueIntervalMs) * time.Millisecond
			if interval > 0 {
				log.Printf("[tg_queue] throttling %v before next job", interval)
				time.Sleep(interval)
			}
		}
	}
}

// WaSend enqueues a WhatsApp send through the rate-limited queue.
// It blocks until the message has been sent and returns the result.
// Use this everywhere instead of waClient.SendMessage directly.
func WaSend(ctx context.Context, jid waTypes.JID, msg *waE2E.Message) (whatsmeow.SendResponse, error) {
	type result struct {
		r whatsmeow.SendResponse
		e error
	}
	ch := make(chan result, 1)
	qDepth := len(waJobCh)
	log.Printf("[wa_queue] enqueuing send to %s (queue depth before enqueue: %d/%d)", jid.String(), qDepth, QueueSize)
	waJobCh <- func() {
		r, e := state.State.WhatsAppClient.SendMessage(ctx, jid, msg)
		ch <- result{r, e}
	}
	res := <-ch
	if res.e != nil {
		log.Printf("[wa_queue] send to %s failed: %v", jid.String(), res.e)
	} else {
		log.Printf("[wa_queue] send to %s succeeded (msgID: %s)", jid.String(), res.r.ID)
	}
	return res.r, res.e
}

// TgRun enqueues any Telegram API call through the rate-limited queue.
// It blocks until the call completes and returns the result.
// Use this everywhere instead of calling bot.SendMessage / SendPhoto / etc. directly.
//
// Example:
//
//	sentMsg, err := queue.TgRun(func() (*gotgbot.Message, error) {
//	    return tgBot.SendMessage(chatId, text, opts)
//	})
func TgRun[T any](fn func() (T, error)) (T, error) {
	type result struct {
		v T
		e error
	}
	ch := make(chan result, 1)
	qDepth := len(tgJobCh)
	log.Printf("[tg_queue] enqueuing job (queue depth before enqueue: %d/%d)", qDepth, QueueSize)
	tgJobCh <- func() {
		v, e := fn()
		ch <- result{v, e}
	}
	res := <-ch
	return res.v, res.e
}

// ---------------------------------------------------------------------------
// Typed convenience wrappers – call sites only change method prefix to queue.Tg
// ---------------------------------------------------------------------------

// TgReopenForumTopic enqueues a Telegram ReopenForumTopic call through the rate-limited queue.
// Corrected: returns (bool, error) to match gotgbot.Bot.ReopenForumTopic
func TgReopenForumTopic(b *gotgbot.Bot, chatId int64, threadId int64, opts *gotgbot.ReopenForumTopicOpts) (bool, error) {
	return TgRun(func() (bool, error) { return b.ReopenForumTopic(chatId, threadId, opts) })
}

func TgOpenForumTopic(b *gotgbot.Bot, chatId int64, name string, opts *gotgbot.CreateForumTopicOpts) (*gotgbot.ForumTopic, error) {
	return TgRun(func() (*gotgbot.ForumTopic, error) { return b.CreateForumTopic(chatId, name, opts) })
}

func TgEditForumTopic(b *gotgbot.Bot, chatId int64, threadId int64, opts *gotgbot.EditForumTopicOpts) (bool, error) {
	return TgRun(func() (bool, error) { return b.EditForumTopic(chatId, threadId, opts) })
}

func TgSendMessage(b *gotgbot.Bot, chatId int64, text string, opts *gotgbot.SendMessageOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendMessage(chatId, text, opts) })
}

func TgSendPhoto(b *gotgbot.Bot, chatId int64, photo gotgbot.InputFile, opts *gotgbot.SendPhotoOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendPhoto(chatId, photo, opts) })
}

func TgSendVideo(b *gotgbot.Bot, chatId int64, video gotgbot.InputFile, opts *gotgbot.SendVideoOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendVideo(chatId, video, opts) })
}

func TgSendVideoNote(b *gotgbot.Bot, chatId int64, videoNote gotgbot.InputFile, opts *gotgbot.SendVideoNoteOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendVideoNote(chatId, videoNote, opts) })
}

func TgSendAudio(b *gotgbot.Bot, chatId int64, audio gotgbot.InputFile, opts *gotgbot.SendAudioOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendAudio(chatId, audio, opts) })
}

func TgSendVoice(b *gotgbot.Bot, chatId int64, voice gotgbot.InputFile, opts *gotgbot.SendVoiceOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendVoice(chatId, voice, opts) })
}

func TgSendDocument(b *gotgbot.Bot, chatId int64, document gotgbot.InputFile, opts *gotgbot.SendDocumentOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendDocument(chatId, document, opts) })
}

func TgSendSticker(b *gotgbot.Bot, chatId int64, sticker gotgbot.InputFile, opts *gotgbot.SendStickerOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendSticker(chatId, sticker, opts) })
}

func TgSendAnimation(b *gotgbot.Bot, chatId int64, animation gotgbot.InputFile, opts *gotgbot.SendAnimationOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendAnimation(chatId, animation, opts) })
}

func TgSendContact(b *gotgbot.Bot, chatId int64, phoneNumber string, firstName string, opts *gotgbot.SendContactOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendContact(chatId, phoneNumber, firstName, opts) })
}

func TgSendLocation(b *gotgbot.Bot, chatId int64, latitude float64, longitude float64, opts *gotgbot.SendLocationOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.SendLocation(chatId, latitude, longitude, opts) })
}

func TgForwardMessage(b *gotgbot.Bot, chatId int64, fromChatId int64, messageId int64, opts *gotgbot.ForwardMessageOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.ForwardMessage(chatId, fromChatId, messageId, opts) })
}
