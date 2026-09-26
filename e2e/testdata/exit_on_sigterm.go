// This file is added to pkg/types only in the E2E build, through
// `go build -overlay`. A binary built with -cover writes its coverage data
// when it exits normally; the web server has no signal handling, so without
// this a stopped server would lose its coverage.
//
// The cover tool ignores overlays on the packages it instruments, so the file
// goes into a package the E2E build leaves out of -coverpkg. pkg/types has no
// statements, so leaving it out costs no coverage, and cmd/web imports it.

package types

import (
	"os"
	"os/signal"
	"syscall"
)

func init() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, os.Interrupt)

	go func() {
		<-ch
		os.Exit(0)
	}()
}
