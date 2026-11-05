// Package appdata locates the local database shared by the desktop app and
// the CLI.
package appdata

import (
	"fmt"
	"os"
	"path/filepath"
)

// DirName is the name of the data directory inside the user configuration directory.
const DirName = "api-regression-guard"

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
	return filepath.Join(base, DirName), nil
}

// DatabasePath returns the path of the local database file.
func DatabasePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.db"), nil
}
