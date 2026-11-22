package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// The floating world (浮世): follows, themed reactions, haiku comments and the
// single featured day that hangs in a person's tokonoma.

// ReactionKinds are the four themed reactions of the floating world: sakura
// (a falling petal), kintsugi (a golden seam), ma (a held breath) and hanabi
// (a burst of light).
var ReactionKinds = []string{"sakura", "kintsugi", "ma", "hanabi"}

// ErrSelfFollow is returned when a user tries to follow (or un-follow) themselves.
var ErrSelfFollow = errors.New("you cannot follow yourself")

// ValidReactionKind reports whether kind is one of the four themed reactions.
func ValidReactionKind(kind string) bool {
	for _, k := range ReactionKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// feedSelect is the shared projection of the public feed: the memory, the
// author's display name, reaction counts per kind, the viewer's own reactions
// and the comment count.
const feedSelect = `SELECT m.id, m.user_id, m.date, m.title, m.text, m.mood_cluster, m.art_direction,
	m.poster_path, m.video_path, m.narration_path, m.track_path, m.track_url, m.track_meta, m.status, m.created_at, m.updated_at, m.is_public,
	COALESCE(NULLIF(u.name, ''), u.email),
	(SELECT COUNT(1) FROM memory_photos p WHERE p.memory_id = m.id),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.kind = 'sakura'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.kind = 'kintsugi'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.kind = 'ma'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.kind = 'hanabi'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.user_id = ? AND r.kind = 'sakura'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.user_id = ? AND r.kind = 'kintsugi'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.user_id = ? AND r.kind = 'ma'),
	(SELECT COUNT(1) FROM reactions r WHERE r.memory_id = m.id AND r.user_id = ? AND r.kind = 'hanabi'),
	(SELECT COUNT(1) FROM comments c WHERE c.memory_id = m.id)
	FROM memories m JOIN users u ON u.id = m.user_id
	WHERE m.is_public = 1`

// queryFeed runs the public feed query with the social filters. followsOnly
// restricts the feed to the viewer's own days plus the days of the people they
// follow; authorID and memoryID add exact filters.
func (s *Store) queryFeed(ctx context.Context, viewerID int64, followsOnly bool, authorID, memoryID int64, limit, offset int) ([]PublicPost, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	if offset < 0 {
		offset = 0
	}
	q := feedSelect
	args := []any{viewerID, viewerID, viewerID, viewerID}
	if followsOnly {
		q += ` AND (m.user_id = ? OR m.user_id IN (SELECT followee_id FROM follows WHERE follower_id = ?))`
		args = append(args, viewerID, viewerID)
	}
	if authorID > 0 {
		q += ` AND m.user_id = ?`
		args = append(args, authorID)
	}
	if memoryID > 0 {
		q += ` AND m.id = ?`
		args = append(args, memoryID)
	}
	q += ` ORDER BY m.date DESC, m.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicPost{}
	for rows.Next() {
		var m Memory
		var art, meta []byte
		var author string
		var photos, cSakura, cKintsugi, cMa, cHanabi, comments int
		var mySakura, myKintsugi, myMa, myHanabi int
		if err := rows.Scan(&m.ID, &m.UserID, &m.Date, &m.Title, &m.Text, &m.MoodCluster, &art,
			&m.PosterPath, &m.VideoPath, &m.NarrationPath, &m.TrackPath, &m.TrackURL, &meta, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.IsPublic,
			&author, &photos, &cSakura, &cKintsugi, &cMa, &cHanabi,
			&mySakura, &myKintsugi, &myMa, &myHanabi, &comments); err != nil {
			return nil, err
		}
		m.ArtDirection, m.TrackMeta, m.Photos = art, meta, photos
		p := PublicPost{
			Memory:    m,
			Author:    author,
			IsMine:    m.UserID == viewerID,
			AuthorID:  m.UserID,
			Comments:  comments,
			Reactions: map[string]int{"sakura": cSakura, "kintsugi": cKintsugi, "ma": cMa, "hanabi": cHanabi},
		}
		if mySakura > 0 {
			p.MyReactions = append(p.MyReactions, "sakura")
		}
		if myKintsugi > 0 {
			p.MyReactions = append(p.MyReactions, "kintsugi")
		}
		if myMa > 0 {
			p.MyReactions = append(p.MyReactions, "ma")
		}
		if myHanabi > 0 {
			p.MyReactions = append(p.MyReactions, "hanabi")
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPublicFeed lists the public feed newest first with author names,
// reaction counts and comment counts.
func (s *Store) ListPublicFeed(ctx context.Context, viewerID int64, followsOnly bool, authorID int64, limit, offset int) ([]PublicPost, error) {
	return s.queryFeed(ctx, viewerID, followsOnly, authorID, 0, limit, offset)
}

// CountPublicFeed counts the feed with the same filters as ListPublicFeed.
func (s *Store) CountPublicFeed(ctx context.Context, viewerID int64, followsOnly bool, authorID int64) (int, error) {
	q := `SELECT COUNT(1) FROM memories m WHERE m.is_public = 1`
	var args []any
	if followsOnly {
		q += ` AND (m.user_id = ? OR m.user_id IN (SELECT followee_id FROM follows WHERE follower_id = ?))`
		args = append(args, viewerID, viewerID)
	}
	if authorID > 0 {
		q += ` AND m.user_id = ?`
		args = append(args, authorID)
	}
	var n int
	err := s.DB.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// PublicPostByID returns one public memory as a feed post (ErrNotFound when
// the memory is missing or not public).
func (s *Store) PublicPostByID(ctx context.Context, viewerID, memoryID int64) (*PublicPost, error) {
	posts, err := s.queryFeed(ctx, viewerID, false, 0, memoryID, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, ErrNotFound
	}
	return &posts[0], nil
}

// ToggleReaction flips the viewer's reaction of the given kind on a public
// memory and returns the new state with the total count of that kind.
func (s *Store) ToggleReaction(ctx context.Context, userID, memoryID int64, kind string) (bool, int, error) {
	if !ValidReactionKind(kind) {
		return false, 0, fmt.Errorf("unknown reaction kind %q", kind)
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM memories WHERE id = ? AND is_public = 1`, memoryID).Scan(&n); err != nil {
		return false, 0, err
	}
	if n == 0 {
		return false, 0, ErrNotFound
	}
	var active int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM reactions WHERE user_id = ? AND memory_id = ? AND kind = ?`, userID, memoryID, kind).Scan(&active); err != nil {
		return false, 0, err
	}
	on := false
	if active == 0 {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO reactions (user_id, memory_id, kind, created_at) VALUES (?, ?, ?, ?)`,
			userID, memoryID, kind, Now()); err != nil {
			return false, 0, err
		}
		on = true
	} else if _, err := s.DB.ExecContext(ctx, `DELETE FROM reactions WHERE user_id = ? AND memory_id = ? AND kind = ?`,
		userID, memoryID, kind); err != nil {
		return false, 0, err
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM reactions WHERE memory_id = ? AND kind = ?`, memoryID, kind).Scan(&count); err != nil {
		return false, 0, err
	}
	return on, count, nil
}

// ReactionDetail returns the per-kind counts and the viewer's own kinds on a
// memory.
func (s *Store) ReactionDetail(ctx context.Context, memoryID, viewerID int64) (map[string]int, []string, error) {
	counts := map[string]int{}
	mine := []string{}
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, COUNT(1), SUM(CASE WHEN user_id = ? THEN 1 ELSE 0 END)
		FROM reactions WHERE memory_id = ? GROUP BY kind`, viewerID, memoryID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var total, mineN int
		if err := rows.Scan(&kind, &total, &mineN); err != nil {
			return nil, nil, err
		}
		counts[kind] = total
		if mineN > 0 {
			mine = append(mine, kind)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for _, k := range ReactionKinds {
		if _, ok := counts[k]; !ok {
			counts[k] = 0
		}
	}
	return counts, mine, nil
}

// SocialComment is one haiku left on a public day.
type SocialComment struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	MemoryID  int64  `json:"memory_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	Author    string `json:"author"`
	IsMine    bool   `json:"mine"`
}

const commentSelect = `SELECT c.id, c.user_id, c.memory_id, c.body, c.created_at,
	COALESCE(NULLIF(u.name, ''), u.email)
	FROM comments c JOIN users u ON u.id = c.user_id`

func scanComment(row interface{ Scan(...any) error }) (*SocialComment, error) {
	var c SocialComment
	if err := row.Scan(&c.ID, &c.UserID, &c.MemoryID, &c.Body, &c.CreatedAt, &c.Author); err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// CommentsFor lists the haiku comments on a memory in the order they were
// left, with author names and a flag for the viewer's own lines.
func (s *Store) CommentsFor(ctx context.Context, memoryID, viewerID int64) ([]SocialComment, error) {
	rows, err := s.DB.QueryContext(ctx, commentSelect+` WHERE c.memory_id = ? ORDER BY c.created_at, c.id`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SocialComment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		c.IsMine = c.UserID == viewerID
		out = append(out, *c)
	}
	return out, rows.Err()
}

// AddComment leaves a haiku on a public day.
func (s *Store) AddComment(ctx context.Context, userID, memoryID int64, body string) (*SocialComment, error) {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM memories WHERE id = ? AND is_public = 1`, memoryID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO comments (user_id, memory_id, body, created_at) VALUES (?, ?, ?, ?)`,
		userID, memoryID, body, Now())
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	c, err := scanComment(s.DB.QueryRowContext(ctx, commentSelect+` WHERE c.id = ?`, id))
	if err != nil {
		return nil, err
	}
	c.IsMine = c.UserID == userID
	return c, nil
}

// DeleteComment removes one of the viewer's own haiku lines (owner only).
func (s *Store) DeleteComment(ctx context.Context, userID, memoryID, commentID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM comments WHERE id = ? AND memory_id = ? AND user_id = ?`,
		commentID, memoryID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFollow adds or removes a follow link; following yourself is refused.
func (s *Store) SetFollow(ctx context.Context, followerID, followeeID int64, on bool) error {
	if followerID == followeeID {
		return ErrSelfFollow
	}
	if on {
		_, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO follows (follower_id, followee_id, created_at) VALUES (?, ?, ?)`,
			followerID, followeeID, Now())
		return err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM follows WHERE follower_id = ? AND followee_id = ?`, followerID, followeeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MiniPost is the latest public day of a person, for their card.
type MiniPost struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Date        string `json:"date"`
	PosterPath  string `json:"poster_path"`
	MoodCluster string `json:"mood_cluster"`
}

// Person is one wanderer of the floating world as seen by the viewer.
type Person struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	PublicPosts int       `json:"public_posts"`
	Followers   int       `json:"followers"`
	Following   int       `json:"following"`
	IFollow     bool      `json:"i_follow"`
	Latest      *MiniPost `json:"latest,omitempty"`
}

const personSelect = `SELECT u.id, COALESCE(NULLIF(u.name, ''), u.email),
	(SELECT COUNT(1) FROM memories m WHERE m.user_id = u.id AND m.is_public = 1),
	(SELECT COUNT(1) FROM follows f WHERE f.followee_id = u.id),
	(SELECT COUNT(1) FROM follows f WHERE f.follower_id = u.id),
	(SELECT COUNT(1) FROM follows f WHERE f.follower_id = ? AND f.followee_id = u.id),
	COALESCE((SELECT m.id FROM memories m WHERE m.user_id = u.id AND m.is_public = 1 ORDER BY m.date DESC, m.id DESC LIMIT 1), 0)
	FROM users u`

// ListPeople lists every wanderer with at least one public day, with follow
// counts and each person's latest public post.
func (s *Store) ListPeople(ctx context.Context, viewerID int64) ([]Person, error) {
	rows, err := s.DB.QueryContext(ctx, personSelect+`
		WHERE (SELECT COUNT(1) FROM memories m WHERE m.user_id = u.id AND m.is_public = 1) > 0
		ORDER BY 2`, viewerID)
	if err != nil {
		return nil, err
	}
	out := []Person{}
	latestID := map[int64]int64{}
	for rows.Next() {
		var p Person
		var ifollow, latest int64
		if err := rows.Scan(&p.ID, &p.Name, &p.PublicPosts, &p.Followers, &p.Following, &ifollow, &latest); err != nil {
			rows.Close()
			return nil, err
		}
		p.IFollow = ifollow == 1
		out = append(out, p)
		latestID[p.ID] = latest
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(latestID) > 0 {
		ids := make([]any, 0, len(latestID))
		for _, id := range latestID {
			if id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
			lrows, err := s.DB.QueryContext(ctx, `SELECT id, title, date, poster_path, mood_cluster FROM memories WHERE id IN (`+marks+`)`, ids...)
			if err != nil {
				return nil, err
			}
			latests := map[int64]*MiniPost{}
			for lrows.Next() {
				p := MiniPost{}
				if err := lrows.Scan(&p.ID, &p.Title, &p.Date, &p.PosterPath, &p.MoodCluster); err != nil {
					lrows.Close()
					return nil, err
				}
				latests[p.ID] = &p
			}
			lerr := lrows.Err()
			lrows.Close()
			if lerr != nil {
				return nil, lerr
			}
			for i := range out {
				out[i].Latest = latests[latestID[out[i].ID]]
			}
		}
	}
	return out, nil
}

// GetPerson returns one wanderer's card (ErrNotFound when the account has no
// public days).
func (s *Store) GetPerson(ctx context.Context, viewerID, id int64) (*Person, error) {
	var p Person
	var ifollow, latest int64
	err := s.DB.QueryRowContext(ctx, personSelect+` WHERE u.id = ?`, viewerID, id).
		Scan(&p.ID, &p.Name, &p.PublicPosts, &p.Followers, &p.Following, &ifollow, &latest)
	if err != nil {
		return nil, notFound(err)
	}
	if p.PublicPosts == 0 {
		return nil, ErrNotFound
	}
	p.IFollow = ifollow == 1
	if latest > 0 {
		lp := MiniPost{}
		if err := s.DB.QueryRowContext(ctx, `SELECT id, title, date, poster_path, mood_cluster FROM memories WHERE id = ?`, latest).
			Scan(&lp.ID, &lp.Title, &lp.Date, &lp.PosterPath, &lp.MoodCluster); err == nil {
			p.Latest = &lp
		}
	}
	return &p, nil
}

// SetFeatured sets or clears the viewer's single featured day — the one flower
// that hangs in their tokonoma; the memory must belong to the viewer.
func (s *Store) SetFeatured(ctx context.Context, userID, memoryID int64, on bool) error {
	if !on {
		res, err := s.DB.ExecContext(ctx, `DELETE FROM featured WHERE user_id = ? AND memory_id = ?`, userID, memoryID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM memories WHERE id = ? AND user_id = ?`, memoryID, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO featured (user_id, memory_id, created_at) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET memory_id = excluded.memory_id, created_at = excluded.created_at`,
		userID, memoryID, Now())
	return err
}

// FeaturedPost returns the viewer's featured public day, falling back to their
// latest public day; nil when they have no public days at all.
func (s *Store) FeaturedPost(ctx context.Context, userID, viewerID int64) (*PublicPost, error) {
	var count, memID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(MAX(memory_id), 0) FROM featured WHERE user_id = ?`, userID).Scan(&count, &memID); err != nil {
		return nil, err
	}
	if count > 0 && memID > 0 {
		posts, err := s.queryFeed(ctx, viewerID, false, 0, memID, 1, 0)
		if err != nil {
			return nil, err
		}
		if len(posts) > 0 {
			return &posts[0], nil
		}
	}
	posts, err := s.queryFeed(ctx, viewerID, false, userID, 0, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, nil
	}
	return &posts[0], nil
}
