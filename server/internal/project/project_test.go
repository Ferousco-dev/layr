package project

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memRepo struct {
	rows   []Project
	calls  int
	purged bool
}

func (m *memRepo) Create(_ context.Context, owner, name string, now time.Time) (Project, error) {
	p := Project{ID: "00000000-0000-4000-8000-00000000000" + string(rune('0'+len(m.rows))), UserID: owner, Name: name, CreatedAt: now, UpdatedAt: now}
	m.rows = append(m.rows, p)
	return p, nil
}

func (m *memRepo) Get(context.Context, string, string) (Project, error) {
	m.calls++
	return Project{}, ErrNotFound
}

func (m *memRepo) List(_ context.Context, _ string, after *Cursor, limit int) ([]Project, error) {
	out := append([]Project(nil), m.rows...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memRepo) Rename(context.Context, string, string, string, time.Time) (Project, error) {
	m.calls++
	return Project{}, ErrNotFound
}

func (m *memRepo) Delete(context.Context, string, string, time.Time) error {
	m.calls++
	m.purged = false
	return ErrNotFound
}

func (m *memRepo) Restore(context.Context, string, string) (Project, error) {
	m.calls++
	return Project{}, ErrNotFound
}

func (m *memRepo) PurgeExpired(context.Context, time.Time) error { m.purged = true; return nil }

func TestValidateName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		invalid bool
	}{
		{"  Landing Page  ", "Landing Page", false},
		{"日本語のプロジェクト", "日本語のプロジェクト", false},
		{"", "", true},
		{"     ", "", true},
		{"bad\x00name", "", true},
		{"new\nline", "", true},
		{"\xff\xfe", "", true},
		{strings.Repeat("a", MaxNameRunes), strings.Repeat("a", MaxNameRunes), false},
		{strings.Repeat("a", MaxNameRunes+1), "", true},
		{strings.Repeat("語", MaxNameRunes), strings.Repeat("語", MaxNameRunes), false},
	}
	for _, tc := range cases {
		got, err := ValidateName(tc.in)
		if tc.invalid != errors.Is(err, ErrInvalidName) || got != tc.want {
			t.Fatalf("%q: got %q err %v", tc.in, got, err)
		}
	}
}

func TestNormalizeID(t *testing.T) {
	good, err := NormalizeID("0A1B2C3D-0000-4000-8000-000000000000")
	if err != nil || good != "0a1b2c3d-0000-4000-8000-000000000000" {
		t.Fatalf("good = %q, %v", good, err)
	}
	for _, bad := range []string{"", "abc", "0a1b2c3d00004000800000000000000000", "0a1b2c3d-0000-4000-8000-00000000000g", "0a1b2c3d-0000-4000-8000-0000000000000", "'; DROP TABLE projects;--"} {
		if _, err := NormalizeID(bad); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestInvalidIDNeverReachesRepository(t *testing.T) {
	repo := &memRepo{}
	svc := NewService(repo)

	_, e1 := svc.Get(context.Background(), "u", "nope")
	_, e2 := svc.Rename(context.Background(), "u", "nope", "x")
	e3 := svc.Delete(context.Background(), "u", "nope")
	_, e4 := svc.Restore(context.Background(), "u", "nope")

	if !errors.Is(e1, ErrInvalidID) || !errors.Is(e2, ErrInvalidID) || !errors.Is(e3, ErrInvalidID) || !errors.Is(e4, ErrInvalidID) || repo.calls != 0 {
		t.Fatalf("errors %v %v %v calls %d", e1, e2, e3, repo.calls)
	}
}

func TestCreateStoresTrimmedName(t *testing.T) {
	svc := NewService(&memRepo{})

	p, err := svc.Create(context.Background(), "owner", "  Hello  ")

	if err != nil || p.Name != "Hello" || p.UserID != "owner" {
		t.Fatalf("p = %#v, err = %v", p, err)
	}
	if _, err := svc.Create(context.Background(), "owner", "   "); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v", err)
	}
}

func TestListPaginationBoundsAndNextCursor(t *testing.T) {
	repo := &memRepo{}
	svc := NewService(repo)
	for i := 0; i < 3; i++ {
		_, _ = svc.Create(context.Background(), "o", "p")
	}

	for _, limit := range []int{-1, MaxLimit + 1} {
		if _, err := svc.List(context.Background(), "o", limit, ""); !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("limit %d accepted", limit)
		}
	}
	if _, err := svc.List(context.Background(), "o", 0, "%%%"); !errors.Is(err, ErrInvalidPage) {
		t.Fatal("bad cursor accepted")
	}

	page, err := svc.List(context.Background(), "o", 2, "")
	if err != nil || len(page.Items) != 2 || page.Next == "" {
		t.Fatalf("page = %#v, err = %v", page, err)
	}
	all, _ := svc.List(context.Background(), "o", 5, "")
	if len(all.Items) != 3 || all.Next != "" {
		t.Fatalf("all = %#v", all)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	p := Project{ID: "0a1b2c3d-0000-4000-8000-000000000000", UpdatedAt: time.UnixMicro(1_700_000_000_123_456).UTC()}

	c, err := decodeCursor(encodeCursor(p))

	if err != nil || c.ID != p.ID || !c.UpdatedAt.Equal(p.UpdatedAt) {
		t.Fatalf("c = %#v, err = %v", c, err)
	}
}

type deleteRepo struct {
	memRepo
	purgedBefore time.Time
}

func (d *deleteRepo) Delete(context.Context, string, string, time.Time) error { return nil }

func (d *deleteRepo) PurgeExpired(_ context.Context, before time.Time) error {
	d.purgedBefore = before
	return nil
}

func TestDeletePurgesOnlyProjectsPastRetention(t *testing.T) {
	repo := &deleteRepo{}
	svc := NewService(repo)
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	err := svc.Delete(context.Background(), "owner", "0a1b2c3d-0000-4000-8000-000000000000")

	if err != nil || !repo.purgedBefore.Equal(now.Add(-Retention)) {
		t.Fatalf("err = %v, purgedBefore = %v", err, repo.purgedBefore)
	}
}
