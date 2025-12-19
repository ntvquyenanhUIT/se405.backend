DROP INDEX IF EXISTS idx_notifications_aggregation;
DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_user_created;
DROP TABLE IF EXISTS notifications;

DROP INDEX IF EXISTS idx_device_tokens_user_id;
DROP TABLE IF EXISTS device_tokens;
