package telegram

import (
	"bytes"
	"go.mau.fi/whatsmeow/types"
	"watgbridge/state"
	"watgbridge/queue"
	"watgbridge/utils"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"go.mau.fi/whatsmeow"
	"go.uber.org/zap"
)

// Send WhatsApp profile picture to Telegram topic
func SendWaProfilePicToTopic(jid types.JID, threadId int64, caption string) {
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
	newPictureBytes, err := utils.DownloadFileBytesByURL(pictureInfo.URL)
	if err != nil {
		logger.Warn("Failed to download profile picture", zap.Error(err), zap.String("url", pictureInfo.URL))
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
