-- A day can carry its song as a link to a music service, besides the
-- uploaded file.
ALTER TABLE memories ADD COLUMN track_url TEXT NOT NULL DEFAULT '';
