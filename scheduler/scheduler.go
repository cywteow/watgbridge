package scheduler

import (
	"strings"
	"time"

	"watgbridge/database"
	"watgbridge/queue"
	"watgbridge/state"
	"watgbridge/utils"

	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

// StartTopicCleanupScheduler launches a background goroutine that runs
// cleanupDeletedTopics and then waits for the configured interval before
// running again. Using a post-completion delay (rather than a fixed clock
// interval) guarantees runs never overlap, even when the job takes longer
// than the interval due to rate-limiting.
func StartTopicCleanupScheduler() {
	const interval = 60 * time.Minute
	go func() {
		for {
			cleanupDeletedTopics()
			time.Sleep(interval)
		}
	}()
}

// StartMsgCleanUpScheduler registers a periodic cron job to clean up old messages.
func StartMsgCleanUpScheduler(s *gocron.Scheduler) {
	const intervalMins = 1440 // adjust as needed
	_, _ = s.Every(intervalMins).Minutes().Tag("msg_cleanup").Do(CleanUpMsg)
}

// Clean up message that doesn't has a topic (thread) associated with it anymore, which means the topic has been deleted and the msg_id_pairs entry is orphaned. This can happen when a Telegram topic is deleted but the scheduler hasn't run yet to clean up the database, or if there was an error during cleanup.
func CleanUpMsg() {
	db := state.State.Database
	// This query works for MySQL and PostgreSQL. For SQLite, the syntax is different.
	sql := `DELETE a FROM msg_id_pairs AS a LEFT JOIN chat_thread_pairs AS b ON a.tg_thread_id = b.tg_thread_id WHERE b.tg_thread_id IS NULL;`
	if db != nil {
		result := db.Exec(sql)
		logger := state.State.Logger
		if result.Error != nil {
			if logger != nil {
				logger.Error("[scheduler] failed to clean up orphaned msg_id_pairs", zap.Error(result.Error))
			}
		} else {
			if logger != nil {
				logger.Info("[scheduler] cleaned up orphaned msg_id_pairs", zap.Int64("rows_affected", result.RowsAffected))
			}
		}
	}
}

// cleanupDeletedTopics is the actual cleanup function executed by the scheduler.
func cleanupDeletedTopics() {
	cfg := state.State.Config
	bot := state.State.TelegramBot
	logger := state.State.Logger
	if bot == nil {
		return
	}

	err := utils.WaSyncContacts()
	if err != nil && logger != nil {
		logger.Error("[scheduler] failed to sync WhatsApp contacts", zap.Error(err))
	}

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

		// Probe Telegram: try to reopen the forum topic using the queue wrapper.
		// - nil error or "TOPIC_NOT_MODIFIED" (already open) → topic still exists.
		// - error containing "TOPIC_NOT_FOUND", "TOPIC_ID_INVALID", "MESSAGE_THREAD_INVALID" → topic has been deleted.
		_, probeErr := queue.TgReopenForumTopic(bot, tgChatId, threadId, nil)
		if probeErr == nil || !isTopicNotFound(probeErr) {
			// Topic is still alive;
			if isTopicNotModified(probeErr) {
				utils.SyncTopicNameByChatThreadPair(bot, tgChatId, pair)
			}
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
	return strings.Contains(msg, "TOPIC_NOT_FOUND") || strings.Contains(msg, "TOPIC_ID_INVALID") || strings.Contains(msg, "MESSAGE_THREAD_INVALID")
}

func isTopicNotModified(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "TOPIC_NOT_MODIFIED")
}
