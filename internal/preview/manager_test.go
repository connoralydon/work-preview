package preview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	previews map[string]Preview
	events   []recordedEvent
}

type recordedEvent struct {
	id, eventType, details string
	at                     time.Time
}

func newMemoryStore() *memoryStore { return &memoryStore{previews: map[string]Preview{}} }

func (s *memoryStore) Create(_ context.Context, p Preview) error {
	for _, existing := range s.previews {
		if existing.Prefix == p.Prefix && existing.Status == StatusActive {
			return ErrPrefixConflict
		}
	}
	s.previews[p.ID] = p
	return nil
}

func (s *memoryStore) Active(context.Context) ([]Preview, error) {
	var result []Preview
	for _, p := range s.previews {
		if p.Status == StatusActive {
			result = append(result, p)
		}
	}
	return result, nil
}

func (s *memoryStore) GetActive(_ context.Context, id string) (Preview, error) {
	p, ok := s.previews[id]
	if !ok || p.Status != StatusActive {
		return Preview{}, ErrNotFound
	}
	return p, nil
}

func (s *memoryStore) Touch(_ context.Context, id string, accessedAt, expiresAt time.Time) error {
	p, err := s.GetActive(context.Background(), id)
	if err != nil {
		return err
	}
	p.LastAccessAt, p.ExpiresAt = accessedAt, expiresAt
	s.previews[id] = p
	return nil
}

func (s *memoryStore) SetStatus(_ context.Context, id, status string, _ time.Time) error {
	p, err := s.GetActive(context.Background(), id)
	if err != nil {
		return err
	}
	p.Status = status
	s.previews[id] = p
	return nil
}

func (s *memoryStore) RecordEvent(_ context.Context, id, eventType string, at time.Time, details string) error {
	s.events = append(s.events, recordedEvent{id: id, eventType: eventType, at: at, details: details})
	return nil
}

type fakeReloader struct {
	calls int
	err   error
}

func (r *fakeReloader) Reload(context.Context) error {
	r.calls++
	return r.err
}

func testManager(t *testing.T, now *time.Time) (*Manager, *memoryStore, *fakeReloader) {
	t.Helper()
	root := t.TempDir()
	store := newMemoryStore()
	reloader := &fakeReloader{}
	manager := &Manager{
		Store: store,
		Files: CaddyWriter{
			SnippetDir: filepath.Join(root, "caddy"), LogDir: filepath.Join(root, "logs"),
			Domain: "p.boringbison.xyz", Certificate: "/run/cert.pem", CertificateKey: "/run/key.pem",
		},
		Reloader: reloader,
		TTL:      time.Hour,
		Now:      func() time.Time { return *now },
	}
	return manager, store, reloader
}

func TestCreateWritesAtomicCaddySnippet(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	manager, store, reloader := testManager(t, &now)
	p, err := manager.Create(context.Background(), "feature-42", 3000)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(manager.Files.SnippetPath(p.ID))
	if err != nil {
		t.Fatal(err)
	}
	want := "feature-42.p.boringbison.xyz {\n\ttls \"/run/cert.pem\" \"/run/key.pem\""
	if !strings.Contains(string(content), want) || !strings.Contains(string(content), "reverse_proxy 127.0.0.1:3000") {
		t.Fatalf("unexpected snippet:\n%s", content)
	}
	if !strings.Contains(string(content), `output file "`+manager.Files.LogPath(p.ID)+`"`) {
		t.Fatalf("snippet does not use preview access log:\n%s", content)
	}
	temps, _ := filepath.Glob(filepath.Join(manager.Files.SnippetDir, ".preview-*.tmp"))
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %v", temps)
	}
	if reloader.calls != 1 || store.previews[p.ID].Status != StatusActive {
		t.Fatalf("reloads=%d status=%s", reloader.calls, store.previews[p.ID].Status)
	}
}

func TestCreateValidatesInputAndPrefixConflicts(t *testing.T) {
	now := time.Now().UTC()
	manager, _, _ := testManager(t, &now)
	for _, test := range []struct {
		prefix string
		port   uint32
	}{
		{"bad_prefix", 3000}, {"-bad", 3000}, {"good", 0}, {"good", 65536},
	} {
		if _, err := manager.Create(context.Background(), test.prefix, test.port); err == nil {
			t.Fatalf("Create(%q, %d) unexpectedly succeeded", test.prefix, test.port)
		}
	}
	if _, err := manager.Create(context.Background(), "same", 3000); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), "same", 3001); !errors.Is(err, ErrPrefixConflict) {
		t.Fatalf("got %v, want ErrPrefixConflict", err)
	}
}

func TestSweepUsesAccessLogTrafficToExtendTTL(t *testing.T) {
	created := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	now := created
	manager, store, reloader := testManager(t, &now)
	p, err := manager.Create(context.Background(), "active", 3000)
	if err != nil {
		t.Fatal(err)
	}
	accessed := created.Add(50 * time.Minute)
	if err := os.WriteFile(manager.Files.LogPath(p.ID), []byte("request\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(manager.Files.LogPath(p.ID), accessed, accessed); err != nil {
		t.Fatal(err)
	}
	now = created.Add(70 * time.Minute)
	if err := manager.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	updated := store.previews[p.ID]
	if !updated.LastAccessAt.Equal(accessed) || !updated.ExpiresAt.Equal(accessed.Add(time.Hour)) {
		t.Fatalf("unexpected lease: access=%s expiry=%s", updated.LastAccessAt, updated.ExpiresAt)
	}
	if updated.Status != StatusActive || reloader.calls != 1 {
		t.Fatalf("status=%s reloads=%d", updated.Status, reloader.calls)
	}
}

func TestSweepExpiresAllIdlePreviewsWithOneReload(t *testing.T) {
	created := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	now := created
	manager, store, reloader := testManager(t, &now)
	first, _ := manager.Create(context.Background(), "first", 3000)
	second, _ := manager.Create(context.Background(), "second", 3001)
	now = created.Add(time.Hour)
	if err := manager.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.previews[first.ID].Status != StatusExpired || store.previews[second.ID].Status != StatusExpired {
		t.Fatal("idle previews were not expired")
	}
	if reloader.calls != 3 {
		t.Fatalf("reloads=%d, want two creates plus one expiry batch", reloader.calls)
	}
	for _, p := range []Preview{first, second} {
		if _, err := os.Stat(manager.Files.SnippetPath(p.ID)); !os.IsNotExist(err) {
			t.Fatalf("snippet %s still exists", p.ID)
		}
	}
}

func TestDeleteRestoresSnippetWhenReloadFails(t *testing.T) {
	now := time.Now().UTC()
	manager, store, reloader := testManager(t, &now)
	p, err := manager.Create(context.Background(), "restore", 3000)
	if err != nil {
		t.Fatal(err)
	}
	reloader.err = errors.New("invalid caddy config")
	if err := manager.Delete(context.Background(), p.ID); err == nil {
		t.Fatal("delete unexpectedly succeeded")
	}
	if _, err := os.Stat(manager.Files.SnippetPath(p.ID)); err != nil {
		t.Fatalf("snippet was not restored: %v", err)
	}
	if store.previews[p.ID].Status != StatusActive {
		t.Fatalf("status=%s, want active", store.previews[p.ID].Status)
	}
	if len(store.events) != 1 || store.events[0].eventType != EventReloadFailed || !strings.Contains(store.events[0].details, "invalid caddy config") {
		t.Fatalf("unexpected failure events: %+v", store.events)
	}
}

func TestReconcileRemovesStaleFilesAndRebuildsActiveRoutes(t *testing.T) {
	now := time.Now().UTC()
	manager, store, reloader := testManager(t, &now)
	if err := os.MkdirAll(manager.Files.SnippetDir, 0o750); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(manager.Files.SnippetDir, "stale.caddy")
	if err := os.WriteFile(stale, []byte("stale"), 0o640); err != nil {
		t.Fatal(err)
	}
	p := Preview{ID: "active-id", Prefix: "active", Port: 3000, Status: StatusActive, CreatedAt: now, LastAccessAt: now, ExpiresAt: now.Add(time.Hour)}
	store.previews[p.ID] = p
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale snippet was not removed")
	}
	if _, err := os.Stat(manager.Files.SnippetPath(p.ID)); err != nil {
		t.Fatalf("active snippet not rebuilt: %v", err)
	}
	if reloader.calls != 1 {
		t.Fatalf("reloads=%d", reloader.calls)
	}
}
