-- # ensure the database and all tables use utf8mb4 character set and collation to properly support emojis and special characters
ALTER DATABASE `your_database_name` 
CHARACTER SET utf8mb4 
COLLATE utf8mb4_unicode_ci;

SELECT CONCAT('ALTER TABLE `', TABLE_NAME, '` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;') AS run_this_sql
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'gobot'
AND TABLE_TYPE = 'BASE TABLE';

ALTER TABLE `chat_ephemeral_settings` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `chat_thread_pairs` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `contact_names` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `msg_id_pairs` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;