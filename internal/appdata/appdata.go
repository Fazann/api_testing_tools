// Package appdata locates the local database shared by the desktop app and
// the CLI.
package appdata

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DirName is the name of the data directory inside the user configuration directory.
const DirName = "api-test-kits"

// LegacyDirNames are data directories used under earlier app names, newest
// first. Dir moves the first one that exists to DirName when the renamed app
// starts and DirName does not exist yet.
var LegacyDirNames = []string{"test-tool-kit", "api-regression-guard"}

// EnvDataDir overrides the data directory, for portable installs and testing.
const EnvDataDir = "APIGUARD_DATA_DIR"

// Dir returns where the local database lives.
func Dir() (string, error) {
	if d := os.Getenv(EnvDataDir); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the user configuration directory: %w", err)
	}
	return resolve(base), nil
}

// resolve returns the data directory under base, moving a legacy directory
// into place when only it exists. If the move fails the legacy directory keeps
// being used, so an update never hides existing data.
func resolve(base string) string {
	dir := filepath.Join(base, DirName)
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return dir
	}
	for _, name := range LegacyDirNames {
		legacy := filepath.Join(base, name)
		if fi, err := os.Stat(legacy); err != nil || !fi.IsDir() {
			continue
		}
		if err := os.Rename(legacy, dir); err != nil {
			return legacy
		}
		return dir
	}
	return dir
}

// DatabasePath returns the path of the local database file.
func DatabasePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.db"), nil
}
