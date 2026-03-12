package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"blackchain/internal/mesh"
)

type configDataDirPeek struct {
	DataDir  string `json:"dataDir"`
	DataDir2 string `json:"data_dir"`
}

func peekDataDirFromConfig(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg configDataDirPeek
	if err := json.Unmarshal(b, &cfg); err != nil {
		return ""
	}
	if cfg.DataDir != "" {
		return cfg.DataDir
	}
	if cfg.DataDir2 != "" {
		return cfg.DataDir2
	}
	return ""
}

func usage() {
	fmt.Fprintf(os.Stderr, `blacknetd - BlackChain mesh daemon

Usage:
  blacknetd mesh [--config PATH] [--data DIR]
  blacknetd --help

Commands:
  mesh   Start mesh daemon (default)

Flags (mesh):
  --config PATH   mesh config json (default: config/mesh-node1.json)
  --data   DIR    data dir (default: data/node1)

Examples:
  blacknetd mesh --config config/mesh-node1.json --data data/node1
`)
}

func main() {
	log.SetFlags(log.LstdFlags)

	cmd := "mesh"
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			usage()
			return
		case "mesh":
			cmd = "mesh"
			args = args[1:]
		default:
			if len(args[0]) > 0 && args[0][0] == '-' {
				cmd = "mesh"
			} else {
				fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
				usage()
				os.Exit(2)
			}
		}
	}

	switch cmd {
	case "mesh":
		fs := flag.NewFlagSet("mesh", flag.ExitOnError)
		cfgPath := fs.String("config", "config/mesh-node1.json", "mesh config json path")
		dataDir := fs.String("data", "data/node1", "data directory")
		_ = fs.Parse(args)

		if *dataDir == "data/node1" {
			if fromCfg := peekDataDirFromConfig(*cfgPath); fromCfg != "" {
				*dataDir = fromCfg
			}
		}

		cfg := &mesh.MeshDaemonOptions{
			MeshConfigPath: *cfgPath,
			DataDir:        *dataDir,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		log.Println("[blacknetd] starting mesh daemon")
		log.Printf("[blacknetd] config=%s data=%s", cfg.MeshConfigPath, cfg.DataDir)

		node, err := mesh.StartMeshDaemon(ctx, cfg)
		if err != nil {
			log.Fatal(err)
		}

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		s := <-sig
		log.Println("[blacknetd] shutdown signal received:", s)
		_ = node.Shutdown(ctx)

	default:
		usage()
		os.Exit(2)
	}
}
