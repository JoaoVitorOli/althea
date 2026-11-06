-- Floating Notes. A note's text is sealed with AES-GCM. A deleted note keeps its row, empty, as a
-- tombstone for sync to carry to the other devices (docs/adr/0011-sync.md).
CREATE TABLE notes_items (
    id      TEXT    PRIMARY KEY,
    body    BLOB,                       -- sealed Markdown; NULL once deleted
    created INTEGER NOT NULL,           -- Unix milliseconds
    updated INTEGER NOT NULL,           -- Unix milliseconds of the last change
    opened  INTEGER NOT NULL,           -- Unix milliseconds it last showed in the window
    deleted INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX notes_items_opened ON notes_items (deleted, opened DESC);
