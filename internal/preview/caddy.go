package preview

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

type CaddyWriter struct {
	SnippetDir     string
	LogDir         string
	Domain         string
	Certificate    string
	CertificateKey string
}

func (w CaddyWriter) Write(p Preview) error {
	if err := os.MkdirAll(w.SnippetDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(w.LogDir, 0o750); err != nil {
		return err
	}
	content := fmt.Sprintf(`%s.%s {
	tls %s %s
	log {
		output file %s
		format json
	}
	reverse_proxy 127.0.0.1:%d
}
`, p.Prefix, w.Domain, caddyQuote(w.Certificate), caddyQuote(w.CertificateKey), caddyQuote(w.LogPath(p.ID)), p.Port)
	tmp, err := os.CreateTemp(w.SnippetDir, ".preview-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, w.SnippetPath(p.ID))
}

func (w CaddyWriter) Remove(id string) error {
	err := os.Remove(w.SnippetPath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (w CaddyWriter) SnippetPath(id string) string {
	return filepath.Join(w.SnippetDir, id+".caddy")
}

func (w CaddyWriter) LogPath(id string) string {
	return filepath.Join(w.LogDir, id+".json")
}

func caddyQuote(value string) string { return strconv.Quote(value) }

type Reloader interface {
	Reload(context.Context) error
}

type CommandReloader struct {
	Binary     string
	ConfigFile string
	Address    string
}

func (r CommandReloader) Reload(ctx context.Context) error {
	args := []string{"reload", "--config", r.ConfigFile, "--adapter", "caddyfile"}
	if r.Address != "" {
		args = append(args, "--address", r.Address)
	}
	output, err := exec.CommandContext(ctx, r.Binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload caddy: %w: %s", err, output)
	}
	return nil
}
