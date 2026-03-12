package mesh

// Build metadata.
// These are safe defaults and can be overridden at build time:
//
//	go build -ldflags "-X blackchain/internal/mesh.Version=v0.1.0 -X blackchain/internal/mesh.Commit=$(git rev-parse --short HEAD) -X blackchain/internal/mesh.BuiltUTC=$(date -u +%Y%m%dT%H%M%SZ)"
var (
	Version  = "dev"
	Commit   = "none"
	BuiltUTC = "unknown"
)
