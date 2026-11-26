package minict

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// eventLog emits a structured lifecycle event (GĐ2 mục 7a, issue #168,
// D68). op = event name (create/start/stop/destroy/snapshot/save-template),
// instance = sandbox name, result = ok|error, duration_ms since start.
// Extra attrs (pid, image, ports, ...) are appended after the standard
// fields. The slog JSON handler emits one line per call with time/level
// automatically; request_id is added by the HTTP middleware (server.go)
// on the access log, not here (runtime is also called from autostart /
// reconcile which have no HTTP request context).
func (s *Store) eventLog(op, instance, result string, start time.Time, extra ...any) {
	args := []any{"instance", instance, "op", op, "result", result, "duration_ms", time.Since(start).Milliseconds()}
	args = append(args, extra...)
	s.Log.Info(op, args...)
}

// eventLogErr emits a lifecycle event with result=error and the error
// message. Used on error return paths of lifecycle methods so MD41 can
// parse both ok and error lines.
func (s *Store) eventLogErr(op, instance string, start time.Time, err error, extra ...any) {
	args := []any{"instance", instance, "op", op, "result", "error", "duration_ms", time.Since(start).Milliseconds(), "error", err.Error()}
	args = append(args, extra...)
	s.Log.Warn(op, args...)
}

// Run starts a sandbox detached and returns its handle. minictd contract
// note: the returned ID IS the instance name (docker returns a 12-char
// short ID; the control plane stores it but always addresses sandboxes by
// name, so identity-by-name is lossless here).
func (s *Store) Run(ctx context.Context, spec contract.RunSpec) (string, error) {
	start := time.Now() // GĐ2 mục 7a: create event duration
	if err := validName(spec.Name); err != nil {
		return "", err
	}
	if spec.Image == "" {
		return "", fmt.Errorf("minict.Run: spec.Image is required")
	}
	if err := os.MkdirAll(s.Root, 0755); err != nil {
		return "", err
	}

	unlock, err := s.lockCreate(ctx, spec.Name)
	if err != nil {
		return "", err
	}
	defer unlock()

	dir := s.dir(spec.Name)
	if _, err := s.loadState(spec.Name); err == nil {
		// Instance dir exists. Docker's equivalent: `docker run --name X`
		// errors when a container named X exists in ANY state.
		st := s.alive(mustState(s, spec.Name))
		return "", fmt.Errorf("instance %s already exists (state %s); Remove() it first", spec.Name, st.State)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	// Any failure from here on leaves a half-built dir without state.json:
	// the next Run with the same name would `cp -a` INTO the stale rootfs/
	// and ListByNamePrefix would report a ghost. Wipe it (R4, 2026-11-18).
	// D67: a pasta attach spawned on the rollback path dies with the
	// instance (Setsid pasta would otherwise pin a dead netns and keep
	// holding its host forward binds).
	ok := false
	pastaProcPid := 0
	defer func() {
		if !ok {
			if pastaProcPid > 0 {
				s.killBestEffort(pastaProcPid, 200*time.Millisecond)
			}
			_ = os.RemoveAll(dir)
		}
	}()

	ports, err := webportsOf(&spec)
	if err != nil {
		return "", err
	}
	// S6 (issue #124, D55): refuse webport collisions up front — a second
	// instance declaring a port already held by a RUNNING instance would
	// have its webproxy fail to bind (EADDRINUSE) with only a Note; detect
	// before spawn so the operator gets a clear 409 instead of a broken
	// preview.
	held, err := s.portsHeldBy(spec.Name)
	if err != nil {
		return "", err
	}
	for _, p := range ports {
		if holder, ok := held[p]; ok {
			return "", fmt.Errorf("%w: port %d held by instance %s", ErrPortInUse, p, holder)
		}
	}
	// D67 (issue #165): pasta forwards validated fail-closed BEFORE any
	// state/dir churn (clear 4xx instead of a pasta-side death later),
	// and cross-instance host-bind collisions detected up front (S6
	// posture carried over to pasta forwards).
	fwds, err := parsePastaPorts(spec.PastaPorts)
	if err != nil {
		return "", err
	}
	pastaHeld, err := s.pastaPortsHeldBy(spec.Name)
	if err != nil {
		return "", err
	}
	for _, f := range fwds {
		if holder, ok := pastaHeld[f.key()]; ok {
			return "", fmt.Errorf("%w: pasta host port %s held by instance %s", ErrPortInUse, f.key(), holder)
		}
	}
	// CG1/S2: resolve the cgroup manager once so renderSpec can note the
	// true enforcement posture (cgroup mapping vs fallback).
	// D59 (issue #139): a template carries the seccomp_profile of the saved
	// instance — its saved posture, not just its filesystem (save-template
	// copies spec.json; D56). The request's explicit seccomp_profile wins
	// (operator override); the inherited one is written back into the new
	// instance's spec.json below (writeJSON), so the setns exec enforces
	// the same allowlist the payload got. Name is validName-checked before
	// it becomes a path (R1 parity with rootfs.go).
	if spec.SeccompProfile == "" && strings.HasPrefix(spec.Image, "template:") {
		if arg := strings.TrimPrefix(spec.Image, "template:"); arg != "" && validName(arg) == nil {
			var tspec contract.RunSpec
			if err := s.readJSON(filepath.Join(s.templatesDir(), arg, fSpec), &tspec); err == nil {
				spec.SeccompProfile = tspec.SeccompProfile
			}
		}
	}
	cgAvail := s.cgroupManager().Avail
	// D66 (issue #162): overlayfs rootfs when MINICT_OVERLAY=1. Resolved
	// BEFORE renderSpec (the directive must be in the .mspec the payload
	// reads) and BEFORE prepareRootfs (which prepares the same layout —
	// both derive from dir/image deterministically). A resolve error is a
	// hard 4xx/5xx: a half-materialized overlay instance would boot nothing.
	var ovl *overlayConfig
	if overlayEnabled() {
		ovl, err = s.overlayFor(dir, spec.Image)
		if err != nil {
			return "", err
		}
	}
	lines, notes, err := renderSpec(&spec, ports, cgAvail, ovl)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, fMspec), []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		return "", err
	}
	if err := s.writeJSON(filepath.Join(dir, fSpec), spec); err != nil {
		return "", err
	}

	if err := s.prepareRootfs(ctx, dir, spec.Image, ovl); err != nil {
		return "", err
	}

	// D67: D42 resolver baked PRE-spawn (no mount race; upper/etc for
	// overlay — lower may be a shared zero-copy source). Fail-soft: a
	// note, not an error; any pasta failure below fails the whole Run so
	// no dead resolver survives.
	var gw string
	if len(fwds) > 0 {
		gw = hostDefaultGW("/proc/net/route")
		notes = append(notes, s.bakeResolver(dir, ovl != nil, gw)...)
	}

	// CG1/S2: create the cgroup and set the limits BEFORE the payload
	// spawns; when enforced, spawnPayload uses the migrate-before-exec
	// launcher so the shim and its payload are born inside the cgroup.
	cgDir, cgNotes := s.cgroupPrepare(spec.Name, &spec)
	notes = append(notes, cgNotes...)
	pid, ticks, err := s.spawnPayload(spec.Name, dir, &spec, cgDir)
	if err != nil {
		return "", err
	}
	proxyPIDs, proxyNotes := s.spawnProxies(spec.Name, dir, ports, 2500*time.Millisecond, ovl != nil)
	notes = append(notes, proxyNotes...)

	// D67 (issue #165): attach pasta AFTER the payload is alive, BEFORE
	// the state is saved. Failure = hard error: the R4 rollback kills
	// pasta (pastaProcPid) and wipes the dir — no silent loopback-only
	// instance pretending to have ports (D34), no dead resolver.
	var pastaPID int
	var pastaTicks uint64
	var pastaMs int64
	if len(fwds) > 0 {
		pastaPID, pastaMs, err = s.spawnPasta(spec.Name, dir, pid, fwds, gw)
		if err != nil {
			return "", err
		}
		_, pastaTicks, _ = procStat(pastaPID)
		pastaProcPid = pastaPID
		notes = append(notes, fmt.Sprintf("pasta attach: %s (pid %d, attach %dms)",
			pastaFwdList(fwds), pastaPID, pastaMs))
	}

	now := time.Now().UTC()
	st := &InstanceState{
		Name:       spec.Name,
		State:      StateRunning,
		PID:        pid,
		StartTicks: ticks,
		ProxyPIDs:  proxyPIDs,
		Ports:      ports,
		CreatedAt:  now,
		StartedAt:  now,
		ExecMode:   execMode(s),
		RootfsSrc:  spec.Image,
		Overlay:    ovl != nil,
		PastaPID:   pastaPID,
		PastaTicks: pastaTicks,
		Notes:      notes,
		CgroupPath: s.cgroupInstancePath(spec.Name),
	}
	if err := s.saveState(spec.Name, st); err != nil {
		return "", err
	}
	ok = true
	// G3 (issue #97): route file for the fresh instance (after saveState,
	// outside the ok=false rollback scope — TraefikDir is outside Root).
	s.syncRouteFile(spec.Name, ports, parseLabels(spec.Labels))
	for _, n := range notes {
		s.Log.Info("minictd note", "name", spec.Name, "note", n)
	}
	s.eventLog("create", spec.Name, "ok", start, "pid", pid, "image", spec.Image, "ports", fmt.Sprint(ports))
	return spec.Name, nil
}

func mustState(s *Store, name string) *InstanceState {
	st, err := s.loadState(name)
	if err != nil {
		return &InstanceState{Name: name, State: StateStopped}
	}
	return st
}

func execMode(s *Store) string {
	if s.MinictExecBin != "" {
		if _, err := os.Stat(s.MinictExecBin); err == nil {
			return ExecSetns
		}
	}
	return ExecNewns
}

// Inspect returns the docker-shaped inspect JSON, or contract.ErrNotFound.
// Status mapping: running -> "running"; stopped -> "exited" (docker's
// status for a container whose main process ended).
func (s *Store) Inspect(ctx context.Context, name string) (*contract.ContainerJSON, error) {
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, contract.ErrNotFound
		}
		return nil, err
	}
	st = s.alive(st)

	spec, err := s.loadSpec(name)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: missing spec.json: %w", name, err)
	}

	cj := &contract.ContainerJSON{}
	cj.ID = st.Name
	if st.State == StateRunning {
		cj.State.Status = "running"
		cj.State.Running = true
		cj.State.Pid = st.PID
	} else {
		cj.State.Status = "exited"
	}
	cj.Config.Image = spec.Image
	cj.Config.Labels = parseLabels(spec.Labels)
	cj.Config.User = spec.User
	cj.Config.Cmd = spec.Cmd

	// Mounts from the saved spec (binds + tmpfs), docker-shaped.
	for _, v := range spec.Volumes {
		parts := strings.SplitN(v, ":", 3)
		if len(parts) < 2 || !strings.HasPrefix(parts[0], "/") {
			continue
		}
		m := contract.Mount{Type: "bind", Source: parts[0], Destination: parts[1], RW: true}
		if len(parts) == 3 && parts[2] == "ro" {
			m.RW, m.Mode = false, "ro"
		}
		cj.Mounts = append(cj.Mounts, m)
	}
	for _, t := range spec.Tmpfs {
		dst := t
		if before, _, ok := strings.Cut(t, ":"); ok {
			dst = before
		}
		cj.Mounts = append(cj.Mounts, contract.Mount{Type: "tmpfs", Source: "", Destination: dst, RW: true})
	}

	// No bridge network: loopback-only netns (D17). IPAddress stays "" and
	// BridgeIP() returns "" — see contract.ContainerJSON.
	return cj, nil
}

// Remove force-removes the sandbox (payload SIGKILLed after a short TERM
// grace, proxies killed, instance directory unlinked). Idempotent: a
// missing instance returns nil (docker `rm -f` parity).
func (s *Store) Remove(ctx context.Context, name string) error {
	start := time.Now()
	unlock, err := s.lock(ctx, name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer unlock()

	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_ = s.alive(st)
	if st.State == StateRunning {
		s.killBestEffort(st.PID, 500*time.Millisecond)
	}
	for _, pp := range st.ProxyPIDs {
		s.killBestEffort(pp, 200*time.Millisecond)
	}
	// D67 (issue #165): end the pasta attach with the instance (host
	// forward binds die with it).
	s.stopPasta(st)
	// CG1/S2: reclaim the instance cgroup with the instance itself.
	s.cgroupCleanup(st.CgroupPath)
	if err := os.RemoveAll(s.dir(name)); err != nil {
		return err
	}
	// G3 (issue #97): instance identity is gone — route must go too.
	s.syncRouteFile(name, nil, nil)
	s.eventLog("destroy", name, "ok", start)
	return nil
}

// SaveTemplate snapshots a STOPPED instance's rootfs + spec into the template
// store (<Root>/_templates/<templateName>/) so a subsequent Run with image
// "template:<templateName>" can clone it (GĐ2 mục 2, issue #127, D56). The
// instance MUST be stopped: copying a running rootfs races payload writes
// (fail-closed → ErrInstanceRunning → 409). templateName shares the instance
// name charset (validName).
func (s *Store) SaveTemplate(ctx context.Context, name, templateName string) error {
	start := time.Now()
	if err := validName(name); err != nil {
		return err
	}
	if err := validName(templateName); err != nil {
		return fmt.Errorf("template name: %w", err)
	}
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.ErrNotFound
		}
		return err
	}
	st = s.alive(st)
	if st.State != StateStopped {
		return fmt.Errorf("%w: save-template requires instance %s stopped (state %s)", ErrInstanceRunning, name, st.State)
	}
	if st.Overlay {
		// D66 (issue #162): host-side rootfs is an EMPTY MOUNTPOINT —
		// the overlay mount lives only inside the shim's mount ns. A
		// template built from it would be an empty rootfs that boots
		// nothing: refuse loudly instead of losing data silently.
		// Merging lower+upper (whiteout-aware) into a plain tree is
		// the residual overlay-aware template work. Checked BEFORE the
		// rootfs stat: the refusal is about the design, not the dir.
		return fmt.Errorf("save-template %s: overlay instances not supported yet (D66 residual — rootfs is an overlay mountpoint, host-side copy would be empty)", name)
	}
	srcRootfs := filepath.Join(s.dir(name), "rootfs")
	if _, err := os.Stat(srcRootfs); err != nil {
		return fmt.Errorf("save-template %s: rootfs missing: %w", name, err)
	}
	tdir := filepath.Join(s.templatesDir(), templateName)
	if err := os.MkdirAll(tdir, 0755); err != nil {
		return err
	}
	dstRootfs := filepath.Join(tdir, "rootfs")
	if err := os.RemoveAll(dstRootfs); err != nil {
		return err
	}
	if err := copyTree(ctx, srcRootfs, dstRootfs); err != nil {
		return fmt.Errorf("save-template %s: copy rootfs: %w", name, err)
	}
	specDst := filepath.Join(tdir, fSpec)
	_ = os.Remove(specDst)
	if err := copyTree(ctx, filepath.Join(s.dir(name), fSpec), specDst); err != nil {
		return fmt.Errorf("save-template %s: copy spec: %w", name, err)
	}
	s.eventLog("save-template", name, "ok", start, "template", templateName)
	return nil
}

// Snapshot creates a gzip-compressed tar backup of a STOPPED instance's rootfs
// into <Root>/_backups/<backupName>.tar.gz (GĐ2 mục 3, issue #130, D57). The
// instance MUST be stopped (fail-closed ErrInstanceRunning -> 409 — a running
// rootfs races payload writes). A subsequent Run with image "backup:<backupName>"
// restores from it (extractGzTar, trusted path). backupName passes validName.
func (s *Store) Snapshot(ctx context.Context, name, backupName string) error {
	start := time.Now()
	if err := validName(name); err != nil {
		return err
	}
	if err := validName(backupName); err != nil {
		return fmt.Errorf("backup name: %w", err)
	}
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.ErrNotFound
		}
		return err
	}
	st = s.alive(st)
	if st.State != StateStopped {
		return fmt.Errorf("%w: snapshot requires instance %s stopped (state %s)", ErrInstanceRunning, name, st.State)
	}
	if st.Overlay {
		// D66 (issue #162): host-side rootfs is an EMPTY MOUNTPOINT (the
		// overlay mount lives only inside the shim's mount ns) — tarGzTree
		// would produce an EMPTY archive presented as a valid backup:
		// silent data loss. Refuse loudly; the whiteout-aware
		// lower+upper merge is the residual overlay-aware snapshot work
		// (D57 already notes full-tar vs thin-provision).
		return fmt.Errorf("snapshot %s: overlay instances not supported yet (D66 residual — rootfs is an overlay mountpoint, archive would be empty)", name)
	}
	srcRootfs := filepath.Join(s.dir(name), "rootfs")
	if _, err := os.Stat(srcRootfs); err != nil {
		return fmt.Errorf("snapshot %s: rootfs missing: %w", name, err)
	}
	bdir := s.backupsDir()
	if err := os.MkdirAll(bdir, 0755); err != nil {
		return err
	}
	archive := filepath.Join(bdir, backupName+".tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	if err := tarGzTree(srcRootfs, f); err != nil {
		f.Close()
		_ = os.Remove(archive)
		return fmt.Errorf("snapshot %s: tar.gz: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("snapshot %s: close: %w", name, err)
	}
	s.eventLog("snapshot", name, "ok", start, "backup", backupName)
	return nil
}

// Start boots a stopped instance from its saved spec on the SAME rootfs
// (filesystem changes persist across stop/start — docker parity). A no-op
// when already running (docker.Client.Start contract).
func (s *Store) Start(ctx context.Context, name string) error {
	start := time.Now()
	unlock, err := s.lock(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()

	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.ErrNotFound
		}
		return err
	}
	st = s.alive(st)
	if st.State == StateRunning {
		return nil
	}

	spec, err := s.loadSpec(name)
	if err != nil {
		return fmt.Errorf("start %s: missing spec.json: %w", name, err)
	}
	rootfs := filepath.Join(s.dir(name), "rootfs")
	if _, err := os.Stat(rootfs); err != nil {
		return fmt.Errorf("start %s: rootfs missing: %w", name, err)
	}
	// Re-verify bind sources still exist so a dead bind fails loudly
	// instead of mounting garbage (minict.c dies on failed binds anyway).
	for _, v := range spec.Volumes {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "/") {
			if _, err := os.Stat(parts[0]); err != nil {
				return fmt.Errorf("start %s: bind source %q: %w", name, parts[0], err)
			}
		}
	}

	// S6 (issue #124, D55): re-check webport collisions before re-spawning
	// proxies — another instance may have grabbed a port while this one
	// was stopped. Same fail-closed posture as Run.
	held, err := s.portsHeldBy(name)
	if err != nil {
		return err
	}
	for _, p := range st.Ports {
		if holder, ok := held[p]; ok {
			return fmt.Errorf("%w: port %d held by instance %s", ErrPortInUse, p, holder)
		}
	}
	// D67 (issue #165): pasta forwards validated + collision-checked on
	// re-Start too — another instance may have taken the host bind while
	// this one was stopped (S6 posture carried over).
	fwds, err := parsePastaPorts(spec.PastaPorts)
	if err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	pastaHeld, err := s.pastaPortsHeldBy(name)
	if err != nil {
		return err
	}
	for _, f := range fwds {
		if holder, ok := pastaHeld[f.key()]; ok {
			return fmt.Errorf("%w: pasta host port %s held by instance %s", ErrPortInUse, f.key(), holder)
		}
	}

	// D67: D42 resolver baked PRE-spawn (Run parity; upper/etc for
	// overlay). Fail-soft notes only.
	var gw string
	var pastaNotes []string
	if len(fwds) > 0 {
		gw = hostDefaultGW("/proc/net/route")
		pastaNotes = append(pastaNotes, s.bakeResolver(s.dir(name), st.Overlay, gw)...)
	}

	// CG1/S2: same prepare-before-spawn contract as Run.
	cgDir, cgNotes := s.cgroupPrepare(name, spec)
	pid, ticks, err := s.spawnPayload(name, s.dir(name), spec, cgDir)
	if err != nil {
		return err
	}
	proxyPIDs, proxyNotes := s.spawnProxies(name, s.dir(name), st.Ports, 2500*time.Millisecond, st.Overlay)

	// D67 (issue #165): re-attach pasta on Start (Stop/reaper killed the
	// old one). Fail-closed (D34): a pasta attach failure kills the
	// payload we just spawned and leaves the instance STOPPED — no
	// silent pasta-less instance pretending to have ports; spec+dir
	// survive so a fixed host can Start again.
	var pastaPID int
	var pastaTicks uint64
	if len(fwds) > 0 {
		ppid, pms, perr := s.spawnPasta(name, s.dir(name), pid, fwds, gw)
		if perr != nil {
			s.killBestEffort(pid, 500*time.Millisecond)
			s.cgroupCleanup(cgDir)
			st.State = StateStopped
			st.Exited = true
			st.ExitCode = 0
			st.KilledBy = "pasta-fail"
			if serr := s.saveState(name, st); serr != nil {
				return fmt.Errorf("start %s: pasta attach failed AND state save failed: %v / %v", name, perr, serr)
			}
			return fmt.Errorf("start %s: pasta attach failed (instance stopped): %w", name, perr)
		}
		pastaPID = ppid
		_, pastaTicks, _ = procStat(ppid)
		pastaNotes = append(pastaNotes, fmt.Sprintf("pasta attach: %s (pid %d, attach %dms)",
			pastaFwdList(fwds), pastaPID, pms))
	}

	st.State = StateRunning
	st.PID = pid
	st.StartTicks = ticks
	st.ProxyPIDs = proxyPIDs
	st.PastaPID = pastaPID
	st.PastaTicks = pastaTicks
	st.StartedAt = time.Now().UTC()
	st.Exited = false
	st.ExitCode = 0
	st.KilledBy = ""
	st.Notes = append(st.Notes, proxyNotes...)
	st.Notes = append(st.Notes, pastaNotes...)
	st.Notes = append(st.Notes, cgNotes...)
	st.CgroupPath = s.cgroupInstancePath(name)
	if err := s.saveState(name, st); err != nil {
		return err
	}
	// G3 (issue #97): route file back on (ports from the saved state).
	s.syncRouteFile(name, st.Ports, parseLabels(spec.Labels))
	s.eventLog("start", name, "ok", start, "pid", pid)
	return nil
}

// Stop terminates the payload (TERM, then SIGKILL after the grace period)
// and keeps the instance directory for a later Start. ErrNotFound on a
// missing instance (docker.Client.Stop contract).
func (s *Store) Stop(ctx context.Context, name string, timeoutSec int) error {
	start := time.Now()
	unlock, err := s.lock(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()

	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.ErrNotFound
		}
		return err
	}
	st = s.alive(st)
	if st.State != StateRunning {
		return nil // docker stop on a stopped container is a no-op
	}

	grace := 10 * time.Second
	if timeoutSec <= 0 {
		grace = 0 // docker stop --time=0: immediate SIGKILL path
	} else {
		grace = time.Duration(timeoutSec) * time.Second
	}
	s.killBestEffort(st.PID, grace)
	for _, pp := range st.ProxyPIDs {
		s.killBestEffort(pp, 200*time.Millisecond)
	}
	// D67 (issue #165): pasta holds HOST-side listening sockets for its
	// forwards — Stop must end the attach too, or the next Start (or
	// another instance) would bind-fail on the same host port.
	s.stopPasta(st)
	// Best-effort wait so a promptly-exiting payload is reaped before we
	// flip the state (the spawn-time reaper goroutine covers stragglers).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !s.pidAlive(st.PID, st.StartTicks) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// CG1/S2: the payload is gone — reclaim the instance cgroup.
	s.cgroupCleanup(st.CgroupPath)
	st.CgroupPath = ""

	st.State = StateStopped
	st.PID = 0
	st.StartTicks = 0
	st.ProxyPIDs = nil
	st.Exited = true
	st.KilledBy = "stop"
	if err := s.saveState(name, st); err != nil {
		return err
	}
	// G3 (issue #97): instance is stopped — its route must go.
	s.syncRouteFile(name, nil, nil)
	s.eventLog("stop", name, "ok", start, "timeout_sec", timeoutSec)
	return nil
}

// Logs serves the merged payload log (the `log` file minict.sh writes).
// tail <= 0 = all lines; follow keeps the reader open until ctx is done,
// the instance dies, or the caller closes (docker.Client.Logs contract).
func (s *Store) Logs(ctx context.Context, name string, tail int, follow bool) (io.ReadCloser, error) {
	if _, err := s.loadState(name); err != nil {
		if os.IsNotExist(err) {
			return nil, contract.ErrNotFound
		}
		return nil, err
	}
	logf := filepath.Join(s.dir(name), fLog)

	if !follow {
		b, err := tailBytes(logf, tail)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(b)), nil
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		// Start at the offset matching the requested tail, then stream
		// growth until ctx done / instance gone / writer closed.
		off, err := startOffset(logf, tail)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		st, _ := s.loadState(name)
		idleDead := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			b, n, err := readFrom(logf, off)
			if err == nil && n > 0 {
				off += int64(n)
				idleDead = 0
				if _, werr := pw.Write(b); werr != nil {
					return // reader closed
				}
			}
			if st != nil && st.State == StateRunning && s.pidAlive(st.PID, st.StartTicks) {
				idleDead = 0
			} else {
				idleDead++
				if idleDead >= 8 { // ~2s with no growth after death
					return
				}
			}
			time.Sleep(250 * time.Millisecond)
		}
	}()
	return pipeCloser{pr}, nil
}

type pipeCloser struct{ *io.PipeReader }

func (p pipeCloser) Close() error { return p.PipeReader.Close() }

// tailBytes returns the last `tail` lines (<=0 → everything).
func tailBytes(path string, tail int) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, err
	}
	if tail <= 0 {
		return b, nil
	}
	lines := bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n"))
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return bytes.Join(lines, []byte("\n")), nil
}

// startOffset returns the byte offset of the start of the last `tail`
// lines (<=0 → 0).
func startOffset(path string, tail int) (int64, error) {
	if tail <= 0 {
		return 0, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	lines := bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n"))
	if len(lines) <= tail {
		return 0, nil
	}
	// offset of the first byte of line[len-tail]
	idx := 0
	for i := 0; i < len(lines)-tail; i++ {
		idx += len(lines[i]) + 1
	}
	return int64(idx), nil
}

func readFrom(path string, off int64) ([]byte, int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if st.Size() <= off {
		return nil, 0, nil
	}
	buf := make([]byte, st.Size()-off)
	n, err := f.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, 0, err
	}
	return buf[:n], n, nil
}

// ListByNamePrefix returns instance names with the prefix, any state
// (docker `ps -a --filter name=^prefix` parity, used by the reconciler).
func (s *Store) ListByNamePrefix(ctx context.Context, prefix string) ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip non-instance dirs (e.g. _templates, GĐ2 mục 2 issue #127):
		// nameRe rejects the "_" prefix; only valid instance names list.
		if !nameRe.MatchString(e.Name()) {
			continue
		}
		if strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Stats samples one resource reading. When cgroups v2 enforcement is on
// (CG1/S2) memory/pids/cpu come from the instance's cgroup accounting
// (memory.current, pids.current, cpu.stat usage over a 250ms window),
// falling back to the /proc RSS tree when the cgroup files are unreadable.
// Net/Block = 0 (loopback-only netns) — documented in contract.Stats.
func (s *Store) Stats(ctx context.Context, name string) (contract.Stats, error) {
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.Stats{}, contract.ErrNotFound
		}
		return contract.Stats{}, err
	}
	st = s.alive(st)

	var out contract.Stats
	spec, err := s.loadSpec(name)
	if err != nil {
		return out, err
	}
	if spec.Memory != "" {
		out.MemLimitBytes, _ = parseDockerSize(spec.Memory)
	}
	if out.MemLimitBytes == 0 {
		out.MemLimitBytes, _ = memInfo()
	}

	if st.State == StateRunning {
		if st.CgroupPath != "" {
			if mem, pids, cpu, ok := cgStats(st.CgroupPath, 250*time.Millisecond); ok {
				out.MemUsageBytes = mem
				out.PIDs = pids
				out.CPUPercent = cpu
				if lim, ok2 := cgReadInt(filepath.Join(st.CgroupPath, "memory.max")); ok2 && lim > 0 {
					out.MemLimitBytes = lim
				}
			} else {
				out.CPUPercent = cpuSample(st.PID, 250*time.Millisecond)
				out.MemUsageBytes = rssTree(st.PID)
				out.PIDs = len(descendants(st.PID))
			}
		} else {
			out.CPUPercent = cpuSample(st.PID, 250*time.Millisecond)
			out.MemUsageBytes = rssTree(st.PID)
			out.PIDs = len(descendants(st.PID))
		}
	}
	if out.MemLimitBytes > 0 {
		out.MemPercent = float64(out.MemUsageBytes) / float64(out.MemLimitBytes) * 100.0
	}
	return out, nil
}

// ServerVersion returns an opaque version string for telemetry (docker
// parity: `docker version --format {{.Server.Version}}`).
func (s *Store) ServerVersion(ctx context.Context) (string, error) {
	return fmt.Sprintf("minictd/%s (minict rootless, exec=%s)", contract.Version, execMode(s)), nil
}

// Info is the readyz probe (docker parity: `docker info --format
// {{.ServerVersion}}`).
func (s *Store) Info(ctx context.Context) (string, error) {
	return s.ServerVersion(ctx)
}

// NetworkSubnets returns the defensive stub subnet for the control plane's
// netguard. minict netns is loopback-only (D17): sandboxes cannot reach
// host IPs at all, but keeping a deny-list entry costs nothing and guards
// against future slirp/bridge work (G3).
func (s *Store) NetworkSubnets(ctx context.Context, network string) (subnets, gateways []string, err error) {
	if s.SandboxSubnet != "" {
		subnets = append(subnets, s.SandboxSubnet)
	}
	if s.SandboxGateway != "" {
		gateways = append(gateways, s.SandboxGateway)
	}
	return subnets, gateways, nil
}

// RawState exposes minictd's own operational state (non-contract extra:
// notes, proxy pids, exec mode, rootfs source). Used by GET .../state.
func (s *Store) RawState(name string) (*InstanceState, error) {
	st, err := s.loadState(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, contract.ErrNotFound
		}
		return nil, err
	}
	return s.alive(st), nil
}

// dialTimeout is a thin wrapper so tests/other callers can stub it.
func dialTimeout(addr string, d time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, d)
}
