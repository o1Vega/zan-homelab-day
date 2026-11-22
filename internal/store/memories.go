package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Memory is a remembrance of one day: the person's text, the atmosphere the
// system developed from it (art direction), and the rendered assets that live
// in the day's folder on disk.
type Memory struct {
	ID            int64           `json:"id"`
	UserID        int64           `json:"user_id"`
	Date          string          `json:"date"`
	Title         string          `json:"title"`
	Text          string          `json:"text"`
	MoodCluster   string          `json:"mood_cluster"`
	ArtDirection  json.RawMessage `json:"art_direction"`
	PosterPath    string          `json:"poster_path"`
	VideoPath     string          `json:"video_path"`
	NarrationPath string          `json:"narration_path"`
	TrackPath     string          `json:"track_path"`
	TrackURL      string          `json:"track_url"`
	TrackMeta     json.RawMessage `json:"track_meta"`
	Status        string          `json:"status"` // draft | analyzing | ready
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`

	IsPublic bool `json:"is_public"` // shared in the public Posts feed

	Photos int `json:"photos,omitempty"` // derived on lists
}

// MemoryPhoto is one photo attached to a day's folder.
type MemoryPhoto struct {
	ID       int64     `json:"id"`
	MemoryID int64     `json:"memory_id"`
	Path     string    `json:"path"`
	Colors   []float64 `json:"colors"` // normalized dominant colors [r,g,b,...]
	Sort     int       `json:"sort"`
}

const memoryCols = `id, user_id, date, title, text, mood_cluster, art_direction, poster_path, video_path, narration_path, track_path, track_url, track_meta, status, created_at, updated_at, is_public`

func scanMemory(row interface{ Scan(...any) error }) (*Memory, error) {
	var m Memory
	var art, meta []byte
	if err := row.Scan(&m.ID, &m.UserID, &m.Date, &m.Title, &m.Text, &m.MoodCluster, &art,
		&m.PosterPath, &m.VideoPath, &m.NarrationPath, &m.TrackPath, &m.TrackURL, &meta, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.IsPublic); err != nil {
		return nil, notFound(err)
	}
	m.ArtDirection = art
	m.TrackMeta = meta
	return &m, nil
}

// GetMemory returns one of the user's memories by id (ErrNotFound when none).
func (s *Store) GetMemory(ctx context.Context, userID, id int64) (*Memory, error) {
	m, err := scanMemory(s.DB.QueryRowContext(ctx, `SELECT `+memoryCols+` FROM memories WHERE id = ? AND user_id = ?`, id, userID))
	if err != nil {
		return nil, err
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM memory_photos WHERE memory_id = ?`, id).Scan(&n); err == nil {
		m.Photos = n
	}
	return m, nil
}

// GetMemoryByDate returns the user's memory for a date (ErrNotFound when none).
func (s *Store) GetMemoryByDate(ctx context.Context, userID int64, date string) (*Memory, error) {
	return scanMemory(s.DB.QueryRowContext(ctx, `SELECT `+memoryCols+` FROM memories WHERE user_id = ? AND date = ?`, userID, date))
}

// UpsertMemory inserts or updates the memory for (user, date). A memory with
// no text, title, photos or track is deleted, like an empty entry.
func (s *Store) UpsertMemory(ctx context.Context, m *Memory) (*Memory, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM memory_photos WHERE memory_id = (SELECT id FROM memories WHERE user_id = ? AND date = ?)`, m.UserID, m.Date).Scan(&n); err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.Text) == "" && strings.TrimSpace(m.Title) == "" && n == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE user_id = ? AND date = ?`, m.UserID, m.Date); err != nil {
			return nil, err
		}
		_ = tx.Commit()
		return &Memory{UserID: m.UserID, Date: m.Date}, nil
	}
	if m.Status == "" {
		m.Status = "draft"
	}
	if m.CreatedAt == "" {
		m.CreatedAt = Now()
	}
	m.UpdatedAt = Now()
	if stringOrEmpty(m.ArtDirection) == "" {
		m.ArtDirection = []byte("{}")
	}
	if stringOrEmpty(m.TrackMeta) == "" {
		m.TrackMeta = []byte("{}")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO memories (user_id, date, title, text, mood_cluster, art_direction, poster_path, video_path, narration_path, track_path, track_url, track_meta, status, created_at, updated_at, is_public)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			title = excluded.title, text = excluded.text, mood_cluster = excluded.mood_cluster,
			art_direction = excluded.art_direction, poster_path = excluded.poster_path,
			video_path = excluded.video_path, narration_path = excluded.narration_path,
			track_path = excluded.track_path, track_url = excluded.track_url, track_meta = excluded.track_meta,
			status = excluded.status, updated_at = excluded.updated_at,
			is_public = excluded.is_public`,
		m.UserID, m.Date, m.Title, strings.TrimRight(m.Text, " \n\t"), m.MoodCluster, stringOrEmpty(m.ArtDirection),
		m.PosterPath, m.VideoPath, m.NarrationPath, m.TrackPath, m.TrackURL, stringOrEmpty(m.TrackMeta), m.Status, m.CreatedAt, m.UpdatedAt, m.IsPublic)
	if err != nil {
		return nil, err
	}
	got, err := scanMemory(tx.QueryRowContext(ctx, `SELECT `+memoryCols+` FROM memories WHERE user_id = ? AND date = ?`, m.UserID, m.Date))
	if err != nil {
		return nil, err
	}
	got.Photos = n
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return got, nil
}

// DeleteMemory removes a memory; the on-disk folder is removed by the caller.
func (s *Store) DeleteMemory(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM memories WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMemories lists the user's memories newest first with photo counts.
func (s *Store) ListMemories(ctx context.Context, userID int64) ([]Memory, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT m.`+memoryCols+`, (SELECT COUNT(1) FROM memory_photos p WHERE p.memory_id = m.id) FROM memories m WHERE m.user_id = ? ORDER BY m.date DESC, m.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var m Memory
		var art, meta []byte
		if err := rows.Scan(&m.ID, &m.UserID, &m.Date, &m.Title, &m.Text, &m.MoodCluster, &art,
			&m.PosterPath, &m.VideoPath, &m.NarrationPath, &m.TrackPath, &m.TrackURL, &meta, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.IsPublic, &m.Photos); err != nil {
			return nil, err
		}
		m.ArtDirection, m.TrackMeta = art, meta
		out = append(out, m)
	}
	return out, rows.Err()
}

// PublicPost is one entry of the public Posts feed: a shared memory with the
// author's name and whether the post belongs to the viewer.
type PublicPost struct {
	Memory
	Author   string `json:"author"`
	IsMine   bool   `json:"is_mine"`
	AuthorID int64  `json:"author_id"`

	Reactions   map[string]int `json:"reactions,omitempty"`
	MyReactions []string       `json:"my_reactions,omitempty"`
	Comments    int            `json:"comments,omitempty"`
}

// CountPublicMemories counts the whole public feed.
func (s *Store) CountPublicMemories(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM memories WHERE is_public = 1`).Scan(&n)
	return n, err
}

// SetMemoryPublic toggles the public flag of a memory (owner only).
func (s *Store) SetMemoryPublic(ctx context.Context, userID, id int64, isPublic bool) error {
	v := 0
	if isPublic {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE memories SET is_public = ?, updated_at = ? WHERE id = ? AND user_id = ?`, v, Now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMemoryByID returns any memory by id without the owner filter — it backs
// the public asset serving in the Posts feed.
func (s *Store) GetMemoryByID(ctx context.Context, id int64) (*Memory, error) {
	m, err := scanMemory(s.DB.QueryRowContext(ctx, `SELECT `+memoryCols+` FROM memories WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM memory_photos WHERE memory_id = ?`, id).Scan(&n); err == nil {
		m.Photos = n
	}
	return m, nil
}

// ListPublicMemories lists the public feed newest first (legacy entry point;
// the social-extended query lives in ListPublicFeed).
func (s *Store) ListPublicMemories(ctx context.Context, viewerID int64, limit, offset int) ([]PublicPost, error) {
	return s.ListPublicFeed(ctx, viewerID, false, 0, limit, offset)
}

// SetMemoryStatus updates the status and art direction after analysis.
func (s *Store) SetMemoryArtDirection(ctx context.Context, userID, id int64, status string, moodCluster string, art []byte) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE memories SET status = ?, mood_cluster = ?, art_direction = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		status, moodCluster, stringOrEmpty(art), Now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMemoryAsset records one rendered asset path.
func (s *Store) SetMemoryAsset(ctx context.Context, userID, id int64, field, path string) error {
	if field != "poster_path" && field != "video_path" && field != "narration_path" && field != "track_path" {
		return fmt.Errorf("unknown asset field %q", field)
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE memories SET `+field+` = ?, updated_at = ? WHERE id = ? AND user_id = ?`, path, Now(), id, userID)
	return err
}

// ListMemoryPhotos lists the photos of a memory in sort order.
func (s *Store) ListMemoryPhotos(ctx context.Context, memoryID int64) ([]MemoryPhoto, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, memory_id, path, colors, sort FROM memory_photos WHERE memory_id = ? ORDER BY sort, id`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemoryPhoto{}
	for rows.Next() {
		var p MemoryPhoto
		var colors []byte
		if err := rows.Scan(&p.ID, &p.MemoryID, &p.Path, &colors, &p.Sort); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(colors, &p.Colors)
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertMemoryPhoto adds or replaces a photo row for a day's folder.
func (s *Store) UpsertMemoryPhoto(ctx context.Context, p *MemoryPhoto) (*MemoryPhoto, error) {
	var colors []byte
	colors, _ = json.Marshal(p.Colors)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO memory_photos (memory_id, path, colors, sort) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET path = excluded.path, colors = excluded.colors, sort = excluded.sort`,
		p.MemoryID, p.Path, string(colors), p.Sort); err != nil {
		return nil, err
	}
	return p, nil
}

// DeleteMemoryPhoto removes one photo row.
func (s *Store) DeleteMemoryPhoto(ctx context.Context, memoryID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM memory_photos WHERE id = ? AND memory_id = ?`, id, memoryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func stringOrEmpty(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return string(b)
}
