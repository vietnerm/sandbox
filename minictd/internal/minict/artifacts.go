package minict

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Artifact is one saved template (<Root>/_templates/<name>/) or backup
// (<Root>/_backups/<name>.tar.gz) as listed for the GĐ3 console
// (issue: GĐ3 mục 1 — "tạo từ template, snapshot/restore"). Image is the
// RunSpec.Image value that clones/restores it.
type Artifact struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// ListTemplates lists saved templates (read-only; GĐ2 #128 residual
// "list template"). Entries whose name fails validName are skipped, so a
// stray operator file can never surface as a clonable image.
func (s *Store) ListTemplates() ([]Artifact, error) {
	ents, err := os.ReadDir(s.templatesDir())
	if os.IsNotExist(err) {
		return []Artifact{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Artifact{}
	for _, e := range ents {
		if !e.IsDir() || validName(e.Name()) != nil {
			continue
		}
		info, err := os.Stat(filepath.Join(s.templatesDir(), e.Name(), fSpec))
		if err != nil {
			continue // half-written template (no spec yet)
		}
		out = append(out, Artifact{
			Name:      e.Name(),
			Image:     "template:" + e.Name(),
			SizeBytes: dirSize(filepath.Join(s.templatesDir(), e.Name(), "rootfs")),
			CreatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ListBackups lists snapshot archives written by Snapshot (GĐ2 #130).
func (s *Store) ListBackups() ([]Artifact, error) {
	ents, err := os.ReadDir(s.backupsDir())
	if os.IsNotExist(err) {
		return []Artifact{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Artifact{}
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".tar.gz")
		if !ok || !e.Type().IsRegular() || validName(name) != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Artifact{
			Name:      name,
			Image:     "backup:" + name,
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RemoveTemplate deletes a saved template directory (<Root>/_templates/<name>/).
// Idempotent: a missing template returns nil (docker rm parity). The name is
// validated and the resolved path must stay under _templates/ after symlink
// resolution — a symlink planted by another uid cannot redirect the delete
// outside the template store (GĐ3 mục 1, issue #179).
func (s *Store) RemoveTemplate(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	base := s.templatesDir()
	target := filepath.Join(base, name)
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // idempotent
		}
		return err
	}
	if real != base && !strings.HasPrefix(real, base+string(os.PathSeparator)) {
		return fmt.Errorf("template %s resolves outside _templates/", name)
	}
	return os.RemoveAll(real)
}

// RemoveBackup deletes a snapshot archive (<Root>/_backups/<name>.tar.gz).
// Idempotent + symlink-safe (same posture as RemoveTemplate).
func (s *Store) RemoveBackup(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	base := s.backupsDir()
	target := filepath.Join(base, name+".tar.gz")
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // idempotent
		}
		return err
	}
	if real != base && !strings.HasPrefix(real, base+string(os.PathSeparator)) {
		return fmt.Errorf("backup %s resolves outside _backups/", name)
	}
	return os.Remove(real)
}

// dirSize sums regular-file sizes under root without following symlinks
// (WalkDir uses Lstat semantics for entries). Errors are skipped: the
// number is informational only.
func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
