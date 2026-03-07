package isolation

import (
	"testing"
)

func TestCgroupManagerIsAvailable(t *testing.T) {
	cm := NewCgroupManager(false)
	// This may or may not be available depending on the test environment
	// Just ensure it doesn't panic
	_ = cm.IsAvailable()
}

func TestCgroupManagerNew(t *testing.T) {
	cm := NewCgroupManager(true)
	if cm.cgroupRoot != "/sys/fs/cgroup" {
		t.Fatalf("expected /sys/fs/cgroup, got %q", cm.cgroupRoot)
	}
	if !cm.useSystemd {
		t.Fatal("expected useSystemd to be true")
	}
}

func TestCgroupManagerRemoveSystemd(t *testing.T) {
	cm := NewCgroupManager(true)
	// Systemd cleanup is automatic, should be a no-op
	if err := cm.Remove("test-agent"); err != nil {
		t.Fatalf("remove: %v", err)
	}
}
