package store

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryLifecycle(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := &User{Email: "a@b", Role: "admin"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	// an empty day deletes itself
	if _, err := st.UpsertMemory(ctx, &Memory{UserID: u.ID, Date: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetMemoryByDate(ctx, u.ID, "2026-10-01"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty memory should be gone, got %v", err)
	}

	// a written day is kept and upserted once per (user, date)
	m, err := st.UpsertMemory(ctx, &Memory{UserID: u.ID, Date: "2026-10-01", Text: "Августовский вечер у воды."})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 || m.Status != "draft" {
		t.Fatalf("unexpected memory %+v", m)
	}
	art := `{"title":"Вечер у воды","mood_cluster":"quiet_warm"}`
	m.Title = "Вечер у воды"
	m.ArtDirection = []byte(art)
	m.Status = "ready"
	m, err = st.UpsertMemory(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	// partial edits go through read-modify-write, like the API handlers
	got, err := st.GetMemoryByDate(ctx, u.ID, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	got.Text = "another line"
	if _, err := st.UpsertMemory(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetMemoryByDate(ctx, u.ID, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != m.ID || got.Title != "Вечер у воды" || got.Status != "ready" || got.Text != "another line" {
		t.Fatalf("upsert changed the row: %+v", got)
	}
	if string(got.ArtDirection) != art {
		t.Fatalf("art direction lost: %s", got.ArtDirection)
	}

	// photos attach to the memory and cascade on delete
	for i, name := range []string{"photo-1.jpg", "photo-2.jpg"} {
		if _, err := st.UpsertMemoryPhoto(ctx, &MemoryPhoto{MemoryID: m.ID, Path: name, Colors: []float64{0.8, 0.5, 0.2}, Sort: i}); err != nil {
			t.Fatal(err)
		}
	}
	photos, err := st.ListMemoryPhotos(ctx, m.ID)
	if err != nil || len(photos) != 2 || photos[0].Path != "photo-1.jpg" || len(photos[0].Colors) != 3 {
		t.Fatalf("photos = %+v, %v", photos, err)
	}
	if err := st.DeleteMemoryPhoto(ctx, m.ID, photos[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ListMemoryPhotos(ctx, m.ID); err != nil {
		t.Fatal(err)
	}

	// the list counts photos and goes newest first
	if _, err := st.UpsertMemory(ctx, &Memory{UserID: u.ID, Date: "2026-10-05", Text: "second day"}); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListMemories(ctx, u.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list[0].Date != "2026-10-05" || list[1].Date != "2026-10-01" {
		t.Fatalf("order wrong: %+v", list)
	}
	if list[1].Photos != 1 {
		t.Fatalf("photo count = %d, want 1", list[1].Photos)
	}

	// deletion removes the row; assets and status setters guard the owner
	if err := st.SetMemoryAsset(ctx, u.ID, m.ID, "poster_path", "poster.png"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMemoryAsset(ctx, u.ID, m.ID, "bogus_field", "x"); err == nil {
		t.Fatal("want an error for an unknown asset field")
	}
	if err := st.DeleteMemory(ctx, u.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetMemory(ctx, u.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if n, err := st.ListMemoryPhotos(ctx, m.ID); err != nil || len(n) != 0 {
		t.Fatalf("photos should cascade away: %+v, %v", n, err)
	}
}

func TestMemoryOwnershipIsPerUser(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	a := &User{Email: "a@b", Role: "admin"}
	b := &User{Email: "b@b", Role: "member"}
	_ = st.CreateUser(ctx, a)
	_ = st.CreateUser(ctx, b)
	m, err := st.UpsertMemory(ctx, &Memory{UserID: a.ID, Date: "2026-10-01", Text: "mine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetMemory(ctx, b.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user may not read the memory: %v", err)
	}
	if err := st.DeleteMemory(ctx, b.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user may not delete the memory: %v", err)
	}
	if _, err := st.GetMemoryByDate(ctx, a.ID, "2026-10-01"); err != nil {
		t.Fatal(err)
	}
}

func TestPublicPosts(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	a := &User{Email: "a@b", Name: "Ann", Role: "admin"}
	b := &User{Email: "b@b", Name: "Bo", Role: "member"}
	_ = st.CreateUser(ctx, a)
	_ = st.CreateUser(ctx, b)

	ma, err := st.UpsertMemory(ctx, &Memory{UserID: a.ID, Date: "2026-10-02", Title: "shared", Text: "mine, public"})
	if err != nil {
		t.Fatal(err)
	}
	mb, err := st.UpsertMemory(ctx, &Memory{UserID: b.ID, Date: "2026-10-03", Title: "theirs", Text: "theirs, public"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertMemory(ctx, &Memory{UserID: b.ID, Date: "2026-10-04", Title: "hidden", Text: "theirs, private"}); err != nil {
		t.Fatal(err)
	}

	// off by default, toggled by the owner only
	if ma.IsPublic {
		t.Fatalf("a memory starts public: %+v", ma)
	}
	if err := st.SetMemoryPublic(ctx, a.ID, ma.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMemoryPublic(ctx, a.ID, mb.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user may not toggle the flag: %v", err)
	}
	if err := st.SetMemoryPublic(ctx, b.ID, mb.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetMemory(ctx, a.ID, ma.ID)
	if err != nil || !got.IsPublic {
		t.Fatalf("flag not stored: %+v, %v", got, err)
	}
	// an upsert keeps the flag
	if _, err := st.UpsertMemory(ctx, &Memory{UserID: a.ID, Date: "2026-10-02", Title: "shared", Text: "more", Status: "ready", IsPublic: got.IsPublic}); err != nil {
		t.Fatal(err)
	}

	// the feed sees only public days, newest first, with author and is_mine
	posts, err := st.ListPublicMemories(ctx, a.ID, 24, 0)
	if err != nil || len(posts) != 2 {
		t.Fatalf("posts = %+v, %v", posts, err)
	}
	if posts[0].ID != mb.ID || posts[0].Author != "Bo" || posts[0].IsMine {
		t.Fatalf("first post wrong: %+v", posts[0])
	}
	if posts[1].ID != ma.ID || posts[1].Author != "Ann" || !posts[1].IsMine {
		t.Fatalf("second post wrong: %+v", posts[1])
	}
	if n, err := st.CountPublicMemories(ctx); err != nil || n != 2 {
		t.Fatalf("count = %d, %v", n, err)
	}

	// GetMemoryByID ignores the owner and serves the public flag
	m, err := st.GetMemoryByID(ctx, mb.ID)
	if err != nil || m.UserID != b.ID || !m.IsPublic {
		t.Fatalf("get by id = %+v, %v", m, err)
	}
	if err := st.SetMemoryPublic(ctx, a.ID, ma.ID, false); err != nil {
		t.Fatal(err)
	}
	posts, err = st.ListPublicMemories(ctx, a.ID, 24, 0)
	if err != nil || len(posts) != 1 || posts[0].ID != mb.ID {
		t.Fatalf("after unshare: %+v, %v", posts, err)
	}
}
