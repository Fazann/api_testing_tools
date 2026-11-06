package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"api-test-kits/internal/service"
	"api-test-kits/internal/updater"
)

// UpdateProgress is emitted as "update:progress" while an update downloads.
type UpdateProgress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

// updates holds the last fetched manifest so InstallUpdate applies exactly
// the version the user was shown.
type updates struct {
	mu       sync.Mutex
	u        *updater.Updater
	manifest *updater.Manifest
	busy     bool
}

func newUpdates() *updates {
	updater.CleanupOld()
	return &updates{u: updater.New(service.Version)}
}

// CheckForUpdate asks the update feed whether a newer version exists.
func (a *App) CheckForUpdate() (updater.Info, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	info, m, err := a.upd.u.Check(ctx)
	if err == nil {
		a.upd.mu.Lock()
		a.upd.manifest = m
		a.upd.mu.Unlock()
	}
	return info, err
}

// InstallUpdate downloads and installs the version found by CheckForUpdate,
// then restarts the app. User data is untouched.
func (a *App) InstallUpdate() error {
	a.upd.mu.Lock()
	m := a.upd.manifest
	if a.upd.busy {
		a.upd.mu.Unlock()
		return errors.New("an update is already being installed")
	}
	if m == nil || !updater.Newer(m.Version, service.Version) {
		a.upd.mu.Unlock()
		return errors.New("no update available; check for updates first")
	}
	a.upd.busy = true
	a.upd.mu.Unlock()
	defer func() {
		a.upd.mu.Lock()
		a.upd.busy = false
		a.upd.mu.Unlock()
	}()

	method, err := a.upd.u.Apply(a.ctx, m, func(done, total int64) {
		runtime.EventsEmit(a.ctx, "update:progress", UpdateProgress{Done: done, Total: total})
	})
	if err != nil {
		return err
	}
	// Close sockets cleanly before the process goes away.
	a.svc.CloseSockets()
	if method != updater.MethodInstaller {
		exe, err := a.upd.u.Executable()
		if err == nil {
			err = updater.Restart(exe)
		}
		if err != nil {
			return err
		}
	}
	runtime.Quit(a.ctx)
	return nil
}

// OpenReleasePage opens the release notes / downloads page in the browser.
func (a *App) OpenReleasePage(url string) { runtime.BrowserOpenURL(a.ctx, url) }
