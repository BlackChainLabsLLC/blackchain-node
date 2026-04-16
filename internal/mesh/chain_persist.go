package mesh

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

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
	return writeFileAtomicDurable(path, tmp, raw, 0o644)
}

func (c *ProductionChain) loadFromDisk() error {
	exactBlockJSONNameRE := regexp.MustCompile(`^[0-9]+\.json$`)
	dir := c.blockDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
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
		if _, err := fmt.Sscanf(name, "%d.json", &h); err == nil {
			files = append(files, pair{h: h, p: filepath.Join(dir, e.Name())})
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].h < files[j].h })

	maxH := int64(-1)
	if len(files) > 0 {
		maxH = files[len(files)-1].h
	}

	for _, f := range files {
		raw, err := os.ReadFile(f.p)
		if err != nil {
			return err
		}
		var b Block
		if err := json.Unmarshal(raw, &b); err != nil {
			if f.h == maxH {
				if recoveryErr := handleReplayCorruption("block replay", f.p, fmt.Errorf("decode height %d: %w", f.h, err), true); recoveryErr == nil {
					continue
				} else {
					return recoveryErr
				}
			}
			return handleReplayCorruption("block replay", f.p, fmt.Errorf("decode height %d: %w", f.h, err), false)
		}
		if err := c.applyBlockLocked(b); err != nil {
			return fmt.Errorf("replay height %d: %w", b.Height, err)
		}
	}

	return nil
}

func writeFileAtomicDurable(path, tmp string, raw []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return err
	}
	return nil
}

func handleReplayCorruption(kind, path string, cause error, allowBestEffort bool) error {
	quarantined, quarantineErr := quarantineCorruptFile(path)
	if quarantineErr != nil {
		return fmt.Errorf("corruption detected during %s at %s: %v; quarantine failed: %w", kind, path, cause, quarantineErr)
	}

	msg := fmt.Sprintf("corruption detected during %s at %s; quarantined to %s: %v", kind, path, quarantined, cause)
	if allowBestEffort && allowBestEffortCorruptRecovery() {
		log.Printf("[recovery] %s; continuing because BLACKCHAIN_ALLOW_CORRUPT_RECOVERY=1", msg)
		return nil
	}

	return fmt.Errorf("%s; startup halted for operator action", msg)
}

func quarantineCorruptFile(path string) (string, error) {
	base := filepath.Base(path)
	cleanBase := strings.ReplaceAll(base, string(filepath.Separator), "_")
	quarantineDir := filepath.Join(filepath.Dir(path), "quarantine")
	if err := os.MkdirAll(quarantineDir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(quarantineDir, fmt.Sprintf("%s.corrupt.%d", cleanBase, time.Now().UTC().UnixNano()))
	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	if err := syncDir(quarantineDir); err != nil {
		return "", err
	}
	return target, syncDir(filepath.Dir(path))
}

func allowBestEffortCorruptRecovery() bool {
	v := strings.TrimSpace(os.Getenv("BLACKCHAIN_ALLOW_CORRUPT_RECOVERY"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
