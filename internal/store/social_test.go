package store

import (
	"context"
	"errors"
	"testing"
)

func TestSocial(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	a := &User{Email: "a@b", Name: "Ann", Role: "admin"}
	b := &User{Email: "b@b", Name: "Bo", Role: "member"}
	c := &User{Email: "c@c", Name: "Cy", Role: "member"}
	_ = st.CreateUser(ctx, a)
	_ = st.CreateUser(ctx, b)
	_ = st.CreateUser(ctx, c)

	ma, err := st.UpsertMemory(ctx, &Memory{UserID: a.ID, Date: "2026-11-01", Title: "rain", Text: "public rain"})
	if err != nil {
		t.Fatal(err)
	}
	mb, err := st.UpsertMemory(ctx, &Memory{UserID: b.ID, Date: "2026-11-02", Title: "neon", Text: "public neon"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertMemory(ctx, &Memory{UserID: c.ID, Date: "2026-11-03", Title: "hidden", Text: "private"}); err != nil {
		t.Fatal(err)
	}
	_ = st.SetMemoryPublic(ctx, a.ID, ma.ID, true)
	_ = st.SetMemoryPublic(ctx, b.ID, mb.ID, true)

	// follows: b follows a; self-follow is refused; unfollow of a missing link errors
	if err := st.SetFollow(ctx, b.ID, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFollow(ctx, a.ID, a.ID, true); !errors.Is(err, ErrSelfFollow) {
		t.Fatalf("self-follow must be refused: %v", err)
	}
	if err := st.SetFollow(ctx, c.ID, a.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unfollow of a missing link should be not found: %v", err)
	}
	if err := st.SetFollow(ctx, b.ID, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFollow(ctx, b.ID, a.ID, true); err != nil {
		t.Fatal(err)
	}

	// the follows-only feed: b sees their own day plus a's, never c's private one
	posts, err := st.ListPublicFeed(ctx, b.ID, true, 0, 24, 0)
	if err != nil || len(posts) != 2 {
		t.Fatalf("follows feed = %+v, %v", posts, err)
	}
	if posts[0].ID != mb.ID || posts[1].ID != ma.ID {
		t.Fatalf("follows order wrong: %+v", posts)
	}
	if n, err := st.CountPublicFeed(ctx, b.ID, true, 0); err != nil || n != 2 {
		t.Fatalf("follows count = %d, %v", n, err)
	}
	// the open feed still sees every public day
	posts, err = st.ListPublicFeed(ctx, c.ID, false, 0, 24, 0)
	if err != nil || len(posts) != 2 {
		t.Fatalf("open feed = %+v, %v", posts, err)
	}
	// the author filter keeps one person's days
	posts, err = st.ListPublicFeed(ctx, a.ID, false, a.ID, 24, 0)
	if err != nil || len(posts) != 1 || posts[0].ID != ma.ID {
		t.Fatalf("author feed = %+v, %v", posts, err)
	}

	// reactions: b drops a sakura and a kintsugi on a's day; toggle flips back
	on, count, err := st.ToggleReaction(ctx, b.ID, ma.ID, "sakura")
	if err != nil || !on || count != 1 {
		t.Fatalf("sakura on = %v %d %v", on, count, err)
	}
	if _, _, err := st.ToggleReaction(ctx, a.ID, ma.ID, "kintsugi"); err != nil {
		t.Fatal(err)
	}
	detail, err := st.PublicPostByID(ctx, b.ID, ma.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Reactions["sakura"] != 1 || detail.Reactions["kintsugi"] != 1 || detail.Reactions["hanabi"] != 0 {
		t.Fatalf("feed reactions = %+v", detail.Reactions)
	}
	if len(detail.MyReactions) != 1 || detail.MyReactions[0] != "sakura" {
		t.Fatalf("b's own reactions = %+v, want [sakura]", detail.MyReactions)
	}
	counts, mine, err := st.ReactionDetail(ctx, ma.ID, a.ID)
	if err != nil || counts["sakura"] != 1 || counts["kintsugi"] != 1 || len(mine) != 1 || mine[0] != "kintsugi" {
		t.Fatalf("detail = %+v %v %v", counts, mine, err)
	}
	on, count, err = st.ToggleReaction(ctx, b.ID, ma.ID, "sakura")
	if err != nil || on || count != 0 {
		t.Fatalf("sakura off = %v %d %v", on, count, err)
	}
	on, count, err = st.ToggleReaction(ctx, b.ID, ma.ID, "sakura")
	if err != nil || !on || count != 1 {
		t.Fatalf("sakura back on = %v %d %v", on, count, err)
	}
	// unknown kinds and private days are refused
	if _, _, err := st.ToggleReaction(ctx, b.ID, ma.ID, "like"); err == nil {
		t.Fatal("unknown kind must fail")
	}
	cID := c.ID
	hidden, _ := st.GetMemoryByDate(ctx, cID, "2026-11-03")
	if _, _, err := st.ToggleReaction(ctx, b.ID, hidden.ID, "ma"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private day must not take reactions: %v", err)
	}

	// comments: b leaves a haiku on a's day; c cannot delete it
	cm, err := st.AddComment(ctx, b.ID, ma.ID, "petal on the screen\nthe rain writes without sound\nthe screen forgets")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddComment(ctx, b.ID, hidden.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private day must not take comments: %v", err)
	}
	list, err := st.CommentsFor(ctx, ma.ID, a.ID)
	if err != nil || len(list) != 1 || list[0].ID != cm.ID || list[0].Author != "Bo" || list[0].IsMine {
		t.Fatalf("comments = %+v, %v", list, err)
	}
	if err := st.DeleteComment(ctx, a.ID, ma.ID, cm.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("only the author may delete a comment: %v", err)
	}
	detail, err = st.PublicPostByID(ctx, b.ID, ma.ID)
	if err != nil || detail.Comments != 1 {
		t.Fatalf("comment count = %+v, %v", detail, err)
	}
	if err := st.DeleteComment(ctx, b.ID, ma.ID, cm.ID); err != nil {
		t.Fatal(err)
	}

	// people: only a and b appear (c has no public day); b follows a
	people, err := st.ListPeople(ctx, b.ID)
	if err != nil || len(people) != 2 {
		t.Fatalf("people = %+v, %v", people, err)
	}
	var pa, pb *Person
	for i := range people {
		switch people[i].ID {
		case a.ID:
			pa = &people[i]
		case b.ID:
			pb = &people[i]
		}
	}
	if pa == nil || pb == nil {
		t.Fatalf("people missing: %+v", people)
	}
	if pa.PublicPosts != 1 || pa.Latest == nil || pa.Latest.ID != ma.ID {
		t.Fatalf("pa = %+v", pa)
	}
	if pb.IFollow {
		t.Fatalf("pb should not follow itself")
	}
	if pa.Followers != 1 {
		t.Fatalf("pa followers = %d", pa.Followers)
	}
	p, err := st.GetPerson(ctx, b.ID, a.ID)
	if err != nil || p.PublicPosts != 1 || !p.IFollow {
		t.Fatalf("person = %+v, %v", p, err)
	}
	if _, err := st.GetPerson(ctx, b.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("c has no public days: %v", err)
	}

	// featured: b features mb, the fallback chain holds, unsetting clears
	if err := st.SetFeatured(ctx, b.ID, ma.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("only the owner may feature: %v", err)
	}
	if err := st.SetFeatured(ctx, b.ID, mb.ID, true); err != nil {
		t.Fatal(err)
	}
	f, err := st.FeaturedPost(ctx, b.ID, a.ID)
	if err != nil || f == nil || f.ID != mb.ID {
		t.Fatalf("featured = %+v, %v", f, err)
	}
	if err := st.SetFeatured(ctx, b.ID, mb.ID, false); err != nil {
		t.Fatal(err)
	}
	f, err = st.FeaturedPost(ctx, b.ID, a.ID)
	if err != nil || f == nil || f.ID != mb.ID { // fallback: latest public
		t.Fatalf("fallback = %+v, %v", f, err)
	}
	if err := st.SetMemoryPublic(ctx, b.ID, mb.ID, false); err != nil {
		t.Fatal(err)
	}
	if f, err := st.FeaturedPost(ctx, b.ID, a.ID); err != nil || f != nil {
		t.Fatalf("no public days → nil featured: %+v, %v", f, err)
	}
}
