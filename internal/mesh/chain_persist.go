package mesh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		var h int64
		if _, err := fmt.Sscanf(e.Name(), "%d.json", &h); err == nil {
			files = append(files, pair{h: h, p: filepath.Join(dir, e.Name())})
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].h < files[j].h })

	for _, f := range files {
		raw, err := os.ReadFile(f.p)
		if err != nil {
			return err
		}
		var b Block
		if err := json.Unmarshal(raw, &b); err != nil {
			return err
		}
		if err := c.applyBlockLocked(b); err != nil {
			return fmt.Errorf("replay height %d: %w", b.Height, err)
		}
	}

	return nil
}
