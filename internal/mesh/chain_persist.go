package mesh

import (
	"bytes"
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

var exactBlockJSONNameRE = regexp.MustCompile(`^[0-9]+\.json$`)
var exactBlockJSONTmpNameRE = regexp.MustCompile(`^([0-9]+)\.json\.tmp$`)

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
	dir := c.blockDir()
	if err := c.recoverInterruptedBlockWrites(dir); err != nil {
		return err
	}
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
		if err := decodePersistedBlock(raw, f.h, &b); err != nil {
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
			allowBestEffort := f.h == maxH
			return handleReplayCorruption("block replay", f.p, fmt.Errorf("apply height %d: %w", b.Height, err), allowBestEffort)
		}
	}

	return nil
}

func (c *ProductionChain) recoverInterruptedBlockWrites(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range ents {
		match := exactBlockJSONTmpNameRE.FindStringSubmatch(e.Name())
		if len(match) != 2 {
			continue
		}
		var h int64
		if _, err := fmt.Sscanf(match[1], "%d", &h); err != nil {
			continue
		}
		tmpPath := filepath.Join(dir, e.Name())
		finalPath := filepath.Join(dir, fmt.Sprintf("%d.json", h))
		if _, err := os.Stat(finalPath); err == nil {
			_ = os.Remove(tmpPath)
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		raw, err := os.ReadFile(tmpPath)
		if err != nil {
			return err
		}
		var b Block
		if err := decodePersistedBlock(raw, h, &b); err != nil {
			return handleReplayCorruption("interrupted block write recovery", tmpPath, fmt.Errorf("decode tmp height %d: %w", h, err), false)
		}
		if err := os.Rename(tmpPath, finalPath); err != nil {
			return fmt.Errorf("recover tmp block %s: %w", tmpPath, err)
		}
		if err := syncDir(dir); err != nil {
			return err
		}
		log.Printf("[recovery] promoted interrupted block write tmp=%s final=%s height=%d", tmpPath, finalPath, h)
	}
	return nil
}

func decodePersistedBlock(raw []byte, expectedHeight int64, out *Block) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("empty block file")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	if out.Height != expectedHeight {
		return fmt.Errorf("filename/content height mismatch: file=%d block=%d", expectedHeight, out.Height)
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
