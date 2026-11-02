-- AI Chat's saved conversations.
CREATE TABLE ai_conversations (
    id      TEXT    PRIMARY KEY,
    title   TEXT    NOT NULL,
    created INTEGER NOT NULL,           -- Unix milliseconds
    updated INTEGER NOT NULL
);

CREATE INDEX ai_conversations_order ON ai_conversations (updated DESC);

CREATE TABLE ai_messages (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation TEXT    NOT NULL REFERENCES ai_conversations (id) ON DELETE CASCADE,
    assistant    INTEGER NOT NULL,      -- 1 for what the model said
    text         TEXT    NOT NULL,
    created      INTEGER NOT NULL
);

CREATE INDEX ai_messages_conversation ON ai_messages (conversation, id);
