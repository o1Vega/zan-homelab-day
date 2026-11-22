-- Public flag on memories: a day can be shared in the public posts feed.
ALTER TABLE memories ADD COLUMN is_public INTEGER NOT NULL DEFAULT 0;
CREATE INDEX memories_public ON memories(is_public, date DESC);
