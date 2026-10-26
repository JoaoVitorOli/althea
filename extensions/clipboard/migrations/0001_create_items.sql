-- Clipboard history. Contents are sealed with AES-GCM (content, thumb, image); the digest is a
-- keyed hash that finds a content copied again without revealing it.
CREATE TABLE clipboard_items (
    id      TEXT    PRIMARY KEY,
    kind    TEXT    NOT NULL,           -- text, link, color, image, files
    digest  BLOB    NOT NULL UNIQUE,
    content BLOB    NOT NULL,           -- sealed JSON: text, html, files, source app
    thumb   BLOB,                       -- sealed PNG preview of an image
    image   BLOB,                       -- sealed PNG of an image, as copied
    size    INTEGER NOT NULL,           -- characters, bytes or files
    copied  INTEGER NOT NULL,           -- Unix milliseconds of the last copy
    pinned  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX clipboard_items_order ON clipboard_items (pinned DESC, copied DESC);
