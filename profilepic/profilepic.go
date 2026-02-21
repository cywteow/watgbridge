package profilepic

import (
	"bytes"
	"io"
	"net/http"
	"watgbridge/state"
	"watgbridge/queue"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"go.uber.org/zap"
)

// SendWaProfilePicToTopic sends WhatsApp profile picture to a Telegram topic.
func SendWaProfilePicToTopic(jid waTypes.JID, threadId int64, caption string) {
	waClient := state.State.WhatsAppClient
	tgBot := state.State.TelegramBot
	cfg := state.State.Config
	logger := state.State.Logger

	pictureInfo, err := waClient.GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{Preview: false})
	if err != nil {
		logger.Warn("Failed to fetch profile picture info", zap.Error(err), zap.String("jid", jid.String()))
		return
	}
	if pictureInfo == nil || pictureInfo.URL == "" {
		logger.Info("No profile picture info or URL", zap.String("jid", jid.String()))
		return
	}
	resp, err := http.Get(pictureInfo.URL)
	if err != nil {
		logger.Warn("Failed to download profile picture", zap.Error(err), zap.String("url", pictureInfo.URL))
		return
	}
	defer resp.Body.Close()
	newPictureBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read profile picture bytes", zap.Error(err))
		return
	}
	_, errSend := queue.TgSendPhoto(tgBot, cfg.Telegram.TargetChatID, &gotgbot.FileReader{Data: bytes.NewReader(newPictureBytes)}, &gotgbot.SendPhotoOpts{
		MessageThreadId: threadId,
		Caption:         caption,
	})
	if errSend != nil {
		logger.Warn("Failed to send profile picture to Telegram", zap.Error(errSend))
	} else {
		logger.Info("Profile picture sent to Telegram topic", zap.String("jid", jid.String()), zap.Int64("threadId", threadId))
	}
}
