package mesh

import "os"

func mustReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
