-- ArtTech: a memory of a day. The person writes how the day felt, attaches
-- photos and a track, and the system develops the atmosphere into a
-- remembrance: an art direction (palette, technique, caption, voice), a
-- rendered poster, a narration and a track. Everything a day owns lives in
-- a per-day folder on disk (data/days/<date>/<slug>/); these rows point to
-- that folder and keep the metadata that the UI and the archive need.

CREATE TABLE memories (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date            TEXT NOT NULL,
  title           TEXT NOT NULL DEFAULT '',
  text            TEXT NOT NULL DEFAULT '',
  mood_cluster    TEXT NOT NULL DEFAULT '',
  art_direction   TEXT NOT NULL DEFAULT '{}',
  poster_path     TEXT NOT NULL DEFAULT '',
  video_path      TEXT NOT NULL DEFAULT '',
  narration_path  TEXT NOT NULL DEFAULT '',
  track_path      TEXT NOT NULL DEFAULT '',
  track_meta      TEXT NOT NULL DEFAULT '{}',
  status          TEXT NOT NULL DEFAULT 'draft',
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  UNIQUE (user_id, date)
);
CREATE INDEX memories_user_date ON memories(user_id, date DESC);

CREATE TABLE memory_photos (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  memory_id  INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
  path       TEXT NOT NULL,
  colors     TEXT NOT NULL DEFAULT '[]',
  sort       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX memory_photos_memory ON memory_photos(memory_id, sort);
