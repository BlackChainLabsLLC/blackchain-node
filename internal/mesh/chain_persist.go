package mesh

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		base := strings.TrimSuffix(name, ".tmp")
		if !exactBlockJSONNameRE.MatchString(base) {
			continue
		}
		finalPath := filepath.Join(dir, base)
		tmpPath := filepath.Join(dir, name)
		if _, err := os.Stat(finalPath); err == nil {
			_ = os.Remove(tmpPath)
			continue
		}
		raw, err := os.ReadFile(tmpPath)
		if err != nil {
			continue
		}
		var b Block
		if err := json.Unmarshal(raw, &b); err != nil {
			log.Printf("[chain] keeping malformed tmp block file path=%s err=%v", tmpPath, err)
			continue
		}
		if err := os.Rename(tmpPath, finalPath); err != nil {
			log.Printf("[chain] tmp block recover failed src=%s dst=%s err=%v", tmpPath, finalPath, err)
			continue
		}
		log.Printf("[chain] recovered interrupted block write path=%s height=%d", finalPath, b.Height)
	}
	ents, err = os.ReadDir(dir)
	if err != nil {
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
				log.Printf("[chain] skipping malformed tail block height=%d path=%s err=%v", f.h, f.p, err)
				continue
			}
			return fmt.Errorf("replay decode height %d path %s: %w", f.h, f.p, err)
		}
		if err := c.applyBlockLocked(b); err != nil {
			return fmt.Errorf("replay height %d: %w", b.Height, err)
		}
	}

	return nil
}
