// Package queue provides rate-limited send queues for WhatsApp and Telegram
// to prevent flooding their respective API servers.
// All outbound sends should go through WaSend / TgRun (or the typed Tg* wrappers).
package queue

import (
	"context"
	"time"

	"watgbridge/state"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
)

var (
	WaInterval = time.Duration(state.State.Config.WhatsApp.QueueIntervalMs) * time.Millisecond
	TgInterval = time.Duration(state.State.Config.Telegram.QueueIntervalMs) * time.Millisecond
	QueueSize = 1000
)

var waJobCh = make(chan func(), QueueSize)
var tgJobCh = make(chan func(), QueueSize)

// StartWorkers launches the background rate-limited sender goroutines.
// Must be called exactly once at startup (before any sends occur).
func StartWorkers() {
	go waWorker()
	go tgWorker()
}

func waWorker() {
	for job := range waJobCh {
		if state.State.Config.WhatsApp.QueueEnabled {
			job()
			time.Sleep(WaInterval)
		} else {
			job()
		}
	}
}

func tgWorker() {
	for job := range tgJobCh {
		if state.State.Config.Telegram.QueueEnabled {
			job()
			time.Sleep(TgInterval)
		} else {
			job()
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
	waJobCh <- func() {
		r, e := state.State.WhatsAppClient.SendMessage(ctx, jid, msg)
		ch <- result{r, e}
	}
	res := <-ch
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
func TgReopenForumTopic(b *gotgbot.Bot, chatId int64, threadId int64, opts *gotgbot.ReopenForumTopicOpts) (*gotgbot.Message, error) {
	return TgRun(func() (*gotgbot.Message, error) { return b.ReopenForumTopic(chatId, threadId, opts) })
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
