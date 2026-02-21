package utils

import (
	"strings"

	"watgbridge/database"
	"watgbridge/state"

	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

// StartTopicCleanupScheduler registers a periodic cron job that checks every
// configured interval whether each Telegram forum topic (thread) that is
// stored in chat_thread_pairs still exists. If a topic has been deleted from
// the Telegram group, the corresponding records in both chat_thread_pairs and
// msg_id_pairs are removed so the database stays in sync.
func StartTopicCleanupScheduler(s *gocron.Scheduler) {
	       const intervalMins = 1
	       _, _ = s.Every(intervalMins).Minutes().Tag("topic_cleanup").Do(cleanupDeletedTopics)
}

// cleanupDeletedTopics is the actual cleanup function executed by the scheduler.
func cleanupDeletedTopics() {
	cfg := state.State.Config
	bot := state.State.TelegramBot
	logger := state.State.Logger
	logger.Info("[scheduler] running topic cleanup job")
	if bot == nil {
		return
	}
	logger.Info("[scheduler] fetching chat_thread_pairs for topic cleanup",
		zap.Int64("tg_chat_id", cfg.Telegram.TargetChatID),
	)
	tgChatId := cfg.Telegram.TargetChatID

	pairs, err := database.ChatThreadGetAllPairs(tgChatId)
	if err != nil {
		logger.Error("[scheduler] failed to fetch chat_thread_pairs for topic cleanup",
			zap.Error(err),
		)
		return
	}

	for _, pair := range pairs {
		threadId := pair.TgThreadId

		// Skip the "General" topic (thread ID 0 or 1) – those can never be deleted.
		if threadId <= 1 {
			continue
		}
		logger.Info("[scheduler] checking Telegram topic existence",
			zap.Int64("tg_thread_id", threadId),
		)

		// Probe Telegram: try to reopen the forum topic.
		// - nil error or "TOPIC_NOT_MODIFIED" (already open) → topic still exists.
		// - error containing "TOPIC_NOT_FOUND"                → topic has been deleted.
		_, probeErr := bot.ReopenForumTopic(tgChatId, threadId, nil)
		if probeErr == nil || !isTopicNotFound(probeErr) {
			logger.Info("[scheduler] Telegram topic exists, no cleanup needed",
				zap.Int64("tg_thread_id", threadId),
			)
			// Topic is still alive; nothing to do.
			continue
		}

		logger.Info("[scheduler] detected deleted Telegram topic, cleaning up",
			zap.Int64("tg_chat_id", tgChatId),
			zap.Int64("tg_thread_id", threadId),
			zap.String("wa_chat_id", pair.ID),
		)

		// Remove all msg_id_pairs rows belonging to this thread.
		if err := database.MsgIdDeletePairsByThreadId(tgChatId, threadId); err != nil {
			logger.Error("[scheduler] failed to delete msg_id_pairs for deleted topic",
				zap.Int64("tg_thread_id", threadId),
				zap.Error(err),
			)
		}

		// Remove the chat_thread_pairs row itself.
		if err := database.ChatThreadDropPairByTg(tgChatId, threadId); err != nil {
			logger.Error("[scheduler] failed to delete chat_thread_pairs for deleted topic",
				zap.Int64("tg_thread_id", threadId),
				zap.Error(err),
			)
		}
	}
}

// isTopicNotFound returns true if the Telegram API error indicates that the
// forum topic no longer exists.
func isTopicNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "TOPIC_NOT_FOUND") ||
		strings.Contains(msg, "MESSAGE_THREAD_INVALID")
}
