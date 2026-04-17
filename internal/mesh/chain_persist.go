package mesh

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

type StartupMode string

const (
	StartupModeCleanStart      StartupMode = "clean_start"
	StartupModeReplayStart     StartupMode = "replay_start"
	StartupModeSnapshotRestore StartupMode = "snapshot_restore"
	StartupModeCorruptionHalt  StartupMode = "corruption_halt"
)

type StartupRecoveryReport struct {
	Mode          StartupMode
	Height        int64
	Tip           string
	Replayed      int
	QuarantineDir string
	Reason        string
}

func (c *ProductionChain) blockDir() string {
	root := c.persistDir
	if root == "" {
		root = c.dataDir
	}
	if root == "" {
		root = "data"
	}
	return filepath.Join(root, "blocks")
}

func (c *ProductionChain) persistBlockLocked(b Block) error {
	dir := c.blockDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.json", b.Height))
	tmp := path + ".tmp"

	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *ProductionChain) haltMarkerPath() string {
	root := c.persistDir
	if root == "" {
		root = c.dataDir
	}
	if root == "" {
		root = "data"
	}
	return filepath.Join(root, "STARTUP_HALT")
}

func (c *ProductionChain) quarantineRootDir() string {
	root := c.persistDir
	if root == "" {
		root = c.dataDir
	}
	if root == "" {
		root = "data"
	}
	return filepath.Join(root, "quarantine")
}

func (c *ProductionChain) quarantineCorruptState(reason string) (string, error) {
	qdir := filepath.Join(c.quarantineRootDir(), time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(qdir, 0755); err != nil {
		return "", err
	}

	blockDir := c.blockDir()
	if _, err := os.Stat(blockDir); err == nil {
		if err := os.Rename(blockDir, filepath.Join(qdir, "blocks")); err != nil {
			return "", fmt.Errorf("move blocks into quarantine: %w", err)
		}
	}

	snapshotPath := c.snapshotPath()
	if _, err := os.Stat(snapshotPath); err == nil {
		if err := os.Rename(snapshotPath, filepath.Join(qdir, "snapshot.json")); err != nil {
			return "", fmt.Errorf("move snapshot into quarantine: %w", err)
		}
	}

	haltBody := fmt.Sprintf("reason=%s\ntime=%s\nquarantine=%s\noperator_action=inspect_and_restore_or_purge\n",
		reason,
		time.Now().UTC().Format(time.RFC3339),
		qdir,
	)
	if err := os.WriteFile(c.haltMarkerPath(), []byte(haltBody), 0644); err != nil {
		return "", fmt.Errorf("write startup halt marker: %w", err)
	}
	return qdir, nil
}

func (c *ProductionChain) loadFromDisk() (int, error) {
	return c.replayBlocksFromHeightLocked(c.height)
}

func (c *ProductionChain) replayBlocksFromHeightLocked(fromHeight int64) (int, error) {
	exactBlockJSONNameRE := regexp.MustCompile(`^[0-9]+\.json$`)
	dir := c.blockDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	type pair struct {
		h int64
		p string
	}
	var files []pair

	for _, e := range ents {
		name := e.Name()
		if !exactBlockJSONNameRE.MatchString(name) {
			continue
		}
		var h int64
		if _, err := fmt.Sscanf(name, "%d.json", &h); err == nil && h > fromHeight {
			files = append(files, pair{h: h, p: filepath.Join(dir, e.Name())})
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].h < files[j].h })

	maxH := int64(-1)
	if len(files) > 0 {
		maxH = files[len(files)-1].h
	}

	replayed := 0
	for _, f := range files {
		raw, err := os.ReadFile(f.p)
		if err != nil {
			return replayed, err
		}
		var b Block
		if err := json.Unmarshal(raw, &b); err != nil {
			if f.h == maxH {
				log.Printf("[chain] skipping malformed tail block height=%d path=%s err=%v", f.h, f.p, err)
				continue
			}
			return replayed, fmt.Errorf("replay decode height %d path %s: %w", f.h, f.p, err)
		}
		if b.Height != f.h {
			return replayed, fmt.Errorf("replay filename/height mismatch file=%d block=%d path=%s", f.h, b.Height, f.p)
		}
		if err := c.applyBlockLocked(b); err != nil {
			return replayed, fmt.Errorf("replay height %d: %w", b.Height, err)
		}
		replayed++
	}

	return replayed, nil
}

func (c *ProductionChain) RecoverFromDisk() (StartupRecoveryReport, error) {
	report := StartupRecoveryReport{
		Mode:   StartupModeCleanStart,
		Height: c.height,
		Tip:    c.tip,
	}

	if raw, err := os.ReadFile(c.haltMarkerPath()); err == nil {
		report.Mode = StartupModeCorruptionHalt
		report.Reason = string(raw)
		return report, fmt.Errorf("startup halted: operator action required (%s)", c.haltMarkerPath())
	}

	snapshotLoaded, snapReplay, err := c.loadSnapshotFromDiskLocked()
	if err != nil {
		qdir, qerr := c.quarantineCorruptState(err.Error())
		report.Mode = StartupModeCorruptionHalt
		report.Reason = err.Error()
		report.QuarantineDir = qdir
		if qerr != nil {
			return report, fmt.Errorf("snapshot recovery failed: %v (quarantine failed: %w)", err, qerr)
		}
		return report, fmt.Errorf("snapshot recovery failed: %w", err)
	}
	report.Replayed += snapReplay
	if snapshotLoaded {
		report.Mode = StartupModeSnapshotRestore
	}

	blockReplay, err := c.loadFromDisk()
	if err != nil {
		qdir, qerr := c.quarantineCorruptState(err.Error())
		report.Mode = StartupModeCorruptionHalt
		report.Reason = err.Error()
		report.QuarantineDir = qdir
		if qerr != nil {
			return report, fmt.Errorf("block replay failed: %v (quarantine failed: %w)", err, qerr)
		}
		return report, fmt.Errorf("block replay failed: %w", err)
	}
	report.Replayed += blockReplay
	if report.Mode == StartupModeCleanStart && blockReplay > 0 {
		report.Mode = StartupModeReplayStart
	}

	report.Height = c.height
	report.Tip = c.tip
	log.Printf("[startup] mode=%s height=%d replayed=%d", report.Mode, report.Height, report.Replayed)

	return report, nil
}
