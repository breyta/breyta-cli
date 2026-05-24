package cli

import "os"

const (
	publicDirMode  os.FileMode = 0o755
	publicFileMode os.FileMode = 0o644
)

func readExplicitFile(path string) ([]byte, error) {
	return os.ReadFile(path) // #nosec G304,G703 -- CLI file flags intentionally read operator-selected local files.
}

func openExplicitFile(path string) (*os.File, error) {
	return os.Open(path) // #nosec G304 -- CLI file flags intentionally read operator-selected local files.
}

func createExplicitFile(path string) (*os.File, error) {
	return os.Create(path) // #nosec G304 -- CLI file flags intentionally write operator-selected local files.
}

func makePublicDir(path string) error {
	return os.MkdirAll(path, publicDirMode) // #nosec G301 -- CLI workspace/docs/flow outputs are user-owned project artifacts.
}

func writePublicFile(path string, content []byte) error {
	return os.WriteFile(path, content, publicFileMode) // #nosec G306 -- CLI workspace/docs/flow outputs are user-owned project artifacts.
}
