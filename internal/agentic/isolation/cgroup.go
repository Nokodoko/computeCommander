package isolation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CgroupManager manages cgroup v2 resource limits for agent processes.
type CgroupManager struct {
	cgroupRoot string // Usually /sys/fs/cgroup
	useSystemd bool   // Use systemd-run --user instead of direct cgroup manipulation
}

// NewCgroupManager creates a new CgroupManager.
func NewCgroupManager(useSystemd bool) *CgroupManager {
	return &CgroupManager{
		cgroupRoot: "/sys/fs/cgroup",
		useSystemd: useSystemd,
	}
}

// Apply sets up cgroup v2 resource limits for the given agent.
// If useSystemd is true, it uses systemd-run --user --scope instead of direct cgroup writes.
func (cm *CgroupManager) Apply(agentID string, limits ResourceLimits) error {
	if cm.useSystemd {
		return cm.applySystemd(agentID, limits)
	}
	return cm.applyDirect(agentID, limits)
}

// Remove tears down the cgroup for an agent.
func (cm *CgroupManager) Remove(agentID string) error {
	if cm.useSystemd {
		// systemd-run scopes are cleaned up automatically when the process exits
		return nil
	}
	groupPath := filepath.Join(cm.cgroupRoot, "computecommander", agentID)
	return os.RemoveAll(groupPath)
}

// IsAvailable checks if cgroup v2 is available on this system.
func (cm *CgroupManager) IsAvailable() bool {
	// Check if cgroup v2 is mounted
	info, err := os.Stat(filepath.Join(cm.cgroupRoot, "cgroup.controllers"))
	return err == nil && !info.IsDir()
}

// GetUsage reads current resource usage for an agent's cgroup.
func (cm *CgroupManager) GetUsage(agentID string) (*ResourceUsage, error) {
	groupPath := filepath.Join(cm.cgroupRoot, "computecommander", agentID)
	usage := &ResourceUsage{}

	// Read memory usage
	memData, err := os.ReadFile(filepath.Join(groupPath, "memory.current"))
	if err == nil {
		usage.MemoryBytes, _ = strconv.ParseInt(strings.TrimSpace(string(memData)), 10, 64)
	}

	// Read CPU usage
	cpuData, err := os.ReadFile(filepath.Join(groupPath, "cpu.stat"))
	if err == nil {
		for _, line := range strings.Split(string(cpuData), "\n") {
			if strings.HasPrefix(line, "usage_usec ") {
				usage.CPUUsec, _ = strconv.ParseInt(strings.TrimPrefix(line, "usage_usec "), 10, 64)
			}
		}
	}

	// Read PIDs count
	pidData, err := os.ReadFile(filepath.Join(groupPath, "pids.current"))
	if err == nil {
		count, _ := strconv.ParseInt(strings.TrimSpace(string(pidData)), 10, 64)
		usage.PIDs = int(count)
	}

	return usage, nil
}

// ResourceUsage holds current resource consumption.
type ResourceUsage struct {
	MemoryBytes int64 `json:"memory_bytes"`
	CPUUsec     int64 `json:"cpu_usec"`
	PIDs        int   `json:"pids"`
}

func (cm *CgroupManager) applyDirect(agentID string, limits ResourceLimits) error {
	groupPath := filepath.Join(cm.cgroupRoot, "computecommander", agentID)

	if err := os.MkdirAll(groupPath, 0o755); err != nil {
		return fmt.Errorf("create cgroup directory: %w", err)
	}

	// Set memory limit
	if limits.MemoryMB > 0 {
		memBytes := int64(limits.MemoryMB) * 1024 * 1024
		if err := os.WriteFile(
			filepath.Join(groupPath, "memory.max"),
			[]byte(strconv.FormatInt(memBytes, 10)),
			0o644,
		); err != nil {
			return fmt.Errorf("set memory.max: %w", err)
		}
	}

	// Set CPU shares
	if limits.CPUShares > 0 {
		if err := os.WriteFile(
			filepath.Join(groupPath, "cpu.weight"),
			[]byte(strconv.Itoa(limits.CPUShares)),
			0o644,
		); err != nil {
			return fmt.Errorf("set cpu.weight: %w", err)
		}
	}

	// Set PIDs limit
	if limits.MaxProcesses > 0 {
		if err := os.WriteFile(
			filepath.Join(groupPath, "pids.max"),
			[]byte(strconv.Itoa(limits.MaxProcesses)),
			0o644,
		); err != nil {
			return fmt.Errorf("set pids.max: %w", err)
		}
	}

	return nil
}

func (cm *CgroupManager) applySystemd(agentID string, limits ResourceLimits) error {
	args := []string{
		"--user", "--scope",
		fmt.Sprintf("--unit=cc-%s", agentID),
	}

	if limits.MemoryMB > 0 {
		args = append(args, fmt.Sprintf("--property=MemoryMax=%dM", limits.MemoryMB))
	}
	if limits.CPUShares > 0 {
		args = append(args, fmt.Sprintf("--property=CPUWeight=%d", limits.CPUShares))
	}
	if limits.MaxProcesses > 0 {
		args = append(args, fmt.Sprintf("--property=TasksMax=%d", limits.MaxProcesses))
	}

	// This sets up the scope; the actual command to run under it would be appended
	cmd := exec.Command("systemd-run", args...)
	return cmd.Run()
}
