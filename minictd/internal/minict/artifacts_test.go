package minict

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveTemplateIdempotent verifies a missing template returns nil
// (idempotent, docker rm parity).
func TestRemoveTemplateIdempotent(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if err := s.RemoveTemplate("nope"); err != nil {
		t.Errorf("RemoveTemplate(missing) = %v, want nil", err)
	}
}

// TestRemoveTemplateValid verifies a real template dir is deleted.
func TestRemoveTemplateValid(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	tname := "mytpl"
	tdir := filepath.Join(dir, "_templates", tname, "rootfs")
	if err := os.MkdirAll(tdir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write spec.json so it looks like a valid template
	if err := os.WriteFile(filepath.Join(dir, "_templates", tname, "spec.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTemplate(tname); err != nil {
		t.Fatalf("RemoveTemplate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_templates", tname)); !os.IsNotExist(err) {
		t.Errorf("template dir still exists after remove")
	}
}

// TestRemoveTemplateInvalidName verifies validName rejection (path traversal).
func TestRemoveTemplateInvalidName(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	for _, bad := range []string{"", "../etc", "a/b", ".hidden", "name with space"} {
		if err := s.RemoveTemplate(bad); err == nil {
			t.Errorf("RemoveTemplate(%q) = nil, want error", bad)
		}
	}
}

// TestRemoveTemplateSymlinkEscape verifies a symlink pointing outside
// _templates/ is refused (cannot redirect delete outside the store).
func TestRemoveTemplateSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	// Plant a symlink _templates/evil -> /tmp (outside)
	tdir := filepath.Join(dir, "_templates")
	if err := os.MkdirAll(tdir, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside-target")
	if err := os.WriteFile(outside, []byte("SECRET"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(tdir, "evil")); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTemplate("evil"); err == nil {
		t.Errorf("RemoveTemplate(symlink escape) = nil, want error")
	}
	// The outside file must still exist
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside file was deleted by symlink escape!")
	}
}

// TestRemoveBackupIdempotent verifies a missing backup returns nil.
func TestRemoveBackupIdempotent(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if err := s.RemoveBackup("nope"); err != nil {
		t.Errorf("RemoveBackup(missing) = %v, want nil", err)
	}
}

// TestRemoveBackupValid verifies a real backup file is deleted.
func TestRemoveBackupValid(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	bname := "mybak"
	bfile := filepath.Join(dir, "_backups", bname+".tar.gz")
	if err := os.MkdirAll(filepath.Dir(bfile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bfile, []byte("FAKE_TAR"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveBackup(bname); err != nil {
		t.Fatalf("RemoveBackup: %v", err)
	}
	if _, err := os.Stat(bfile); !os.IsNotExist(err) {
		t.Errorf("backup file still exists after remove")
	}
}

// TestRemoveBackupInvalidName verifies validName rejection.
func TestRemoveBackupInvalidName(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	for _, bad := range []string{"", "../etc", "a/b", ".hidden"} {
		if err := s.RemoveBackup(bad); err == nil {
			t.Errorf("RemoveBackup(%q) = nil, want error", bad)
		}
	}
}

// TestRemoveBackupSymlinkEscape verifies a symlink backup pointing outside
// _backups/ is refused.
func TestRemoveBackupSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	bdir := filepath.Join(dir, "_backups")
	if err := os.MkdirAll(bdir, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside-backup")
	if err := os.WriteFile(outside, []byte("SECRET"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(bdir, "evil.tar.gz")); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveBackup("evil"); err == nil {
		t.Errorf("RemoveBackup(symlink escape) = nil, want error")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside file was deleted by symlink escape!")
	}
}
