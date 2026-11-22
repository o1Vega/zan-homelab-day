package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"example.com/zan-homelab-day/internal/store"
)

// The floating world (浮世): the social layer of the public feed. Reactions
// are four gestures, not likes; comments are haiku; each person keeps one
// featured day in their tokonoma.

// handlePostGet returns one public day with its reactions and haiku comments.
func (s *Server) handlePostGet(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad post id")
		return
	}
	post, err := s.store.PublicPostByID(r.Context(), u.ID, id)
	if err != nil {
		s.fail(w, err, "get post")
		return
	}
	counts, mine, err := s.store.ReactionDetail(r.Context(), id, u.ID)
	if err != nil {
		s.fail(w, err, "reactions")
		return
	}
	comments, err := s.store.CommentsFor(r.Context(), id, u.ID)
	if err != nil {
		s.fail(w, err, "comments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"post":      post,
		"reactions": map[string]any{"counts": counts, "mine": mine},
		"comments":  comments,
	})
}

// handlePostReaction toggles the viewer's themed reaction on a public day.
func (s *Server) handlePostReaction(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad post id")
		return
	}
	kind := chi.URLParam(r, "kind")
	if !store.ValidReactionKind(kind) {
		writeErr(w, http.StatusBadRequest, "unknown reaction kind")
		return
	}
	active, count, err := s.store.ToggleReaction(r.Context(), u.ID, id, kind)
	if err != nil {
		s.fail(w, err, "toggle reaction")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "active": active, "count": count})
}

// handlePostComment adds a haiku comment to a public day (up to 200 chars).
func (s *Server) handlePostComment(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad post id")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, `pass {"body": "..."}`)
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		writeErr(w, http.StatusBadRequest, "write a few words")
		return
	}
	if len([]rune(body)) > 200 {
		writeErr(w, http.StatusBadRequest, "haiku is short — up to 200 characters")
		return
	}
	c, err := s.store.AddComment(r.Context(), u.ID, id, body)
	if err != nil {
		s.fail(w, err, "add comment")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// handlePostCommentDelete removes one of the viewer's own haiku lines.
func (s *Server) handlePostCommentDelete(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad post id")
		return
	}
	cid, ok := memID(r, "cid")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad comment id")
		return
	}
	if err := s.store.DeleteComment(r.Context(), u.ID, id, cid); err != nil {
		s.fail(w, err, "delete comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePeopleList lists the wanderers who share at least one day.
func (s *Server) handlePeopleList(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	people, err := s.store.ListPeople(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "list people")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "me": u.ID})
}

// handlePerson returns one wanderer's tokonoma: person, featured day and feed.
func (s *Server) handlePerson(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad person id")
		return
	}
	person, err := s.store.GetPerson(r.Context(), u.ID, id)
	if err != nil {
		s.fail(w, err, "get person")
		return
	}
	featured, err := s.store.FeaturedPost(r.Context(), id, u.ID)
	if err != nil {
		s.fail(w, err, "featured post")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	posts, err := s.store.ListPublicFeed(r.Context(), u.ID, false, id, limit, 0)
	if err != nil {
		s.fail(w, err, "person posts")
		return
	}
	total, _ := s.store.CountPublicFeed(r.Context(), u.ID, false, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"person":   person,
		"featured": featured,
		"posts":    posts,
		"total":    total,
		"me":       u.ID,
	})
}

// handlePersonFollow follows a wanderer (or un-follows on DELETE).
func (s *Server) handlePersonFollow(w http.ResponseWriter, r *http.Request, on bool) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad person id")
		return
	}
	if err := s.store.SetFollow(r.Context(), u.ID, id, on); err != nil {
		if errors.Is(err, store.ErrSelfFollow) {
			writeErr(w, http.StatusBadRequest, "you cannot follow yourself")
			return
		}
		s.fail(w, err, "follow")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"following": on})
}

func (s *Server) handlePersonFollowOn(w http.ResponseWriter, r *http.Request) {
	s.handlePersonFollow(w, r, true)
}
func (s *Server) handlePersonFollowOff(w http.ResponseWriter, r *http.Request) {
	s.handlePersonFollow(w, r, false)
}

// handleMemoryFeatured sets or clears the viewer's featured day.
func (s *Server) handleMemoryFeatured(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	var in struct {
		Featured bool `json:"featured"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, `pass {"featured": true|false}`)
		return
	}
	if _, err := s.store.GetMemory(r.Context(), u.ID, id); err != nil {
		s.fail(w, err, "get memory")
		return
	}
	if err := s.store.SetFeatured(r.Context(), u.ID, id, in.Featured); err != nil {
		s.fail(w, err, "set featured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"featured": in.Featured})
}
