-- The file index: every file and folder under the folders File Search covers, with a trigram
-- index of their names for searching any part of them.
CREATE TABLE files_entries (
    id       INTEGER PRIMARY KEY,
    path     TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    name     TEXT    NOT NULL,
    folder   INTEGER NOT NULL,
    modified INTEGER NOT NULL,   -- Unix milliseconds
    seen     INTEGER NOT NULL    -- the scan that last found it
);

CREATE INDEX files_entries_name ON files_entries (name COLLATE NOCASE);
CREATE INDEX files_entries_modified ON files_entries (modified DESC);

CREATE VIRTUAL TABLE files_names USING fts5(
    name, content = 'files_entries', content_rowid = 'id',
    tokenize = 'trigram remove_diacritics 1'
);

CREATE TRIGGER files_entries_insert AFTER INSERT ON files_entries BEGIN
    INSERT INTO files_names (rowid, name) VALUES (new.id, new.name);
END;

CREATE TRIGGER files_entries_delete AFTER DELETE ON files_entries BEGIN
    INSERT INTO files_names (files_names, rowid, name) VALUES ('delete', old.id, old.name);
END;

CREATE TRIGGER files_entries_rename AFTER UPDATE OF name ON files_entries
WHEN old.name <> new.name BEGIN
    INSERT INTO files_names (files_names, rowid, name) VALUES ('delete', old.id, old.name);
    INSERT INTO files_names (rowid, name) VALUES (new.id, new.name);
END;
