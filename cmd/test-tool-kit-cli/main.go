// Command test-tool-kit-cli runs collections headlessly, for CI pipelines and
// scheduled jobs. It is pure Go and needs no GUI libraries. See docs/cli.md.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"api-test-kits/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := cli.Main(ctx, os.Args[1:], cli.Env{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		Environ: os.Environ,
		Color:   isTerminal(os.Stdout),
	})
	stop()
	os.Exit(code)
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
