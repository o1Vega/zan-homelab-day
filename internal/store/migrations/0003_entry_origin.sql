-- Legacy column kept for schema compatibility: it marked where an
-- entry's words came from. New entries are always the user's own.
ALTER TABLE entries ADD COLUMN origin TEXT NOT NULL DEFAULT 'user';
