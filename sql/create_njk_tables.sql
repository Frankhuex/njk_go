DO $$
BEGIN
    IF current_database() <> 'njk' THEN
        RAISE EXCEPTION 'this script only supports database "njk", current=%', current_database();
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS "user" (
    user_id VARCHAR(30) PRIMARY KEY,
    nickname VARCHAR(100) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_user_nickname
    ON "user" (nickname);

CREATE TABLE IF NOT EXISTS "group" (
    group_id VARCHAR(30) PRIMARY KEY,
    group_name VARCHAR(100) NOT NULL
);

CREATE TABLE IF NOT EXISTS message (
    message_id VARCHAR(30) PRIMARY KEY,
    "time" TIMESTAMP NOT NULL,
    sender_id VARCHAR(30) NULL REFERENCES "user"(user_id) ON DELETE SET NULL,
    group_id VARCHAR(30) NULL REFERENCES "group"(group_id) ON DELETE SET NULL,
    card VARCHAR(100) NULL,
    text TEXT NULL,
    reply_id VARCHAR(30) NULL REFERENCES message(message_id) ON DELETE SET NULL,
    raw_json JSONB NULL,
    raw_message TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_message_time
    ON message ("time");

CREATE INDEX IF NOT EXISTS idx_message_group_id
    ON message (group_id);

CREATE INDEX IF NOT EXISTS idx_message_sender_id
    ON message (sender_id);

CREATE INDEX IF NOT EXISTS idx_message_reply_id
    ON message (reply_id);

CREATE INDEX IF NOT EXISTS idx_message_group_time
    ON message (group_id, "time");

CREATE INDEX IF NOT EXISTS idx_message_sender_time
    ON message (sender_id, "time");

CREATE INDEX IF NOT EXISTS idx_message_group_sender_time
    ON message (group_id, sender_id, "time");

CREATE TABLE IF NOT EXISTS at_user (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(30) NOT NULL REFERENCES message(message_id) ON DELETE CASCADE,
    user_id VARCHAR(30) NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    CONSTRAINT uq_at_user_message_user UNIQUE (message_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_at_user_user_message
    ON at_user (user_id, message_id);

CREATE INDEX IF NOT EXISTS idx_at_user_message_id
    ON at_user (message_id);

CREATE INDEX IF NOT EXISTS idx_at_user_user_id
    ON at_user (user_id);

CREATE TABLE IF NOT EXISTS image (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(30) NOT NULL REFERENCES message(message_id) ON DELETE CASCADE,
    image_hash VARCHAR(100) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_image_message_id
    ON image (message_id);

CREATE INDEX IF NOT EXISTS idx_image_image_hash
    ON image (image_hash);

CREATE TABLE IF NOT EXISTS img_whitelist (
    id SERIAL PRIMARY KEY,
    image_hash VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS topic (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    group_id VARCHAR(30) NULL REFERENCES "group"(group_id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_topic_group_id
    ON topic (group_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_group_name
    ON topic (group_id, name);

CREATE TABLE IF NOT EXISTS word (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    group_id VARCHAR(30) NULL REFERENCES "group"(group_id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_word_group_id
    ON word (group_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_word_group_name
    ON word (group_id, name);

CREATE TABLE IF NOT EXISTS msg_topic (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(30) NOT NULL REFERENCES message(message_id) ON DELETE CASCADE,
    topic_id INTEGER NOT NULL REFERENCES topic(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_msg_topic_message_id
    ON msg_topic (message_id);

CREATE INDEX IF NOT EXISTS idx_msg_topic_topic_id
    ON msg_topic (topic_id);

CREATE INDEX IF NOT EXISTS idx_msg_topic_message_topic
    ON msg_topic (message_id, topic_id);

CREATE TABLE IF NOT EXISTS msg_word (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(30) NOT NULL REFERENCES message(message_id) ON DELETE CASCADE,
    word_id INTEGER NOT NULL REFERENCES word(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_msg_word_message_id
    ON msg_word (message_id);

CREATE INDEX IF NOT EXISTS idx_msg_word_word_id
    ON msg_word (word_id);

CREATE INDEX IF NOT EXISTS idx_msg_word_message_word
    ON msg_word (message_id, word_id);
