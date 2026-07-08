//go:build linux

package wg

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AppMatcher moves the processes of the selected split-tunnel apps into the apps
// cgroup so their egress is fwmark-tagged. Linux has no per-app VPN API, so it polls
// /proc and matches each process by its executable basename (the .desktop Exec id),
// which also captures an app's helper/child processes since they share the binary.
// Re-scanning picks up processes started after connect and forked children.
type AppMatcher struct {
	cg    *CgroupMark
	execs map[string]bool
	stop  chan struct{}
	done  chan struct{}
}

// StartAppMatcher begins polling and adding matching processes to cg.
func StartAppMatcher(cg *CgroupMark, execs []string) *AppMatcher {
	set := make(map[string]bool, len(execs))
	for _, e := range execs {
		if e = strings.TrimSpace(e); e != "" {
			set[e] = true
		}
	}
	m := &AppMatcher{cg: cg, execs: set, stop: make(chan struct{}), done: make(chan struct{})}
	go m.loop()
	return m
}

func (m *AppMatcher) loop() {
	defer close(m.done)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	m.scan()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.scan()
		}
	}
}

func (m *AppMatcher) scan() {
	if len(m.execs) == 0 {
		return
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if m.matches(pid) {
			// Idempotent: re-adding a process already in the cgroup is a no-op; PID
			// reuse is handled because we only ever add currently-matching processes.
			_ = m.cg.Add(pid)
		}
	}
}

func (m *AppMatcher) matches(pid int) bool {
	dir := "/proc/" + strconv.Itoa(pid)
	if target, err := os.Readlink(dir + "/exe"); err == nil {
		if m.execs[filepath.Base(target)] {
			return true
		}
	}
	// comm fallback (process name, truncated to 15 chars by the kernel).
	if b, err := os.ReadFile(dir + "/comm"); err == nil {
		if m.execs[strings.TrimSpace(string(b))] {
			return true
		}
	}
	return false
}

// Stop ends polling and waits for the loop to exit.
func (m *AppMatcher) Stop() {
	if m == nil {
		return
	}
	close(m.stop)
	<-m.done
}
