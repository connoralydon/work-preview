package preview

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var prefixPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Manager struct {
	Store    Store
	Files    CaddyWriter
	Reloader Reloader
	TTL      time.Duration
	Now      func() time.Time
}

func (m *Manager) Create(ctx context.Context, prefix string, port uint32) (Preview, error) {
	if port == 0 || port > 65535 {
		return Preview{}, errors.New("port must be between 1 and 65535")
	}
	if prefix == "" {
		var err error
		prefix, err = randomToken(10)
		if err != nil {
			return Preview{}, err
		}
	}
	prefix = strings.ToLower(prefix)
	if !prefixPattern.MatchString(prefix) {
		return Preview{}, errors.New("prefix must be a valid lowercase DNS label")
	}
	id, err := randomToken(20)
	if err != nil {
		return Preview{}, err
	}
	now := m.now()
	p := Preview{
		ID: id, Prefix: prefix, Port: uint16(port), Status: StatusActive,
		CreatedAt: now, LastAccessAt: now, ExpiresAt: now.Add(m.TTL),
	}
	if err := m.Store.Create(ctx, p); err != nil {
		return Preview{}, err
	}
	if err := m.Files.Write(p); err != nil {
		_ = m.Store.SetStatus(ctx, p.ID, StatusDeleted, now)
		return Preview{}, fmt.Errorf("write caddy snippet: %w", err)
	}
	if err := m.Reloader.Reload(ctx); err != nil {
		_ = m.Files.Remove(p.ID)
		_ = m.Store.SetStatus(ctx, p.ID, StatusDeleted, now)
		return Preview{}, err
	}
	return p, nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	p, err := m.Store.GetActive(ctx, id)
	if err != nil {
		return err
	}
	if err := m.Files.Remove(id); err != nil {
		return err
	}
	if err := m.Reloader.Reload(ctx); err != nil {
		_ = m.Files.Write(p)
		return err
	}
	return m.Store.SetStatus(ctx, id, StatusDeleted, m.now())
}

func (m *Manager) Active(ctx context.Context) ([]Preview, error) {
	return m.Store.Active(ctx)
}

func (m *Manager) Reconcile(ctx context.Context) error {
	previews, err := m.Store.Active(ctx)
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(m.Files.SnippetDir, "*.caddy"))
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	for _, p := range previews {
		if err := m.Files.Write(p); err != nil {
			return err
		}
	}
	return m.Reloader.Reload(ctx)
}

func (m *Manager) Sweep(ctx context.Context) error {
	previews, err := m.Store.Active(ctx)
	if err != nil {
		return err
	}
	now := m.now()
	var expired []Preview
	for _, p := range previews {
		info, statErr := os.Stat(m.Files.LogPath(p.ID))
		if statErr == nil && info.ModTime().After(p.LastAccessAt) {
			p.LastAccessAt = info.ModTime()
			p.ExpiresAt = p.LastAccessAt.Add(m.TTL)
			if err := m.Store.Touch(ctx, p.ID, p.LastAccessAt, p.ExpiresAt); err != nil {
				return err
			}
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if !now.Before(p.ExpiresAt) {
			expired = append(expired, p)
		}
	}
	if len(expired) == 0 {
		return nil
	}
	for _, p := range expired {
		if err := m.Files.Remove(p.ID); err != nil {
			return err
		}
	}
	if err := m.Reloader.Reload(ctx); err != nil {
		for _, p := range expired {
			_ = m.Files.Write(p)
		}
		return err
	}
	for _, p := range expired {
		if err := m.Store.SetStatus(ctx, p.ID, StatusExpired, now); err != nil {
			return err
		}
		_ = os.Remove(m.Files.LogPath(p.ID))
	}
	return nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func randomToken(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}
