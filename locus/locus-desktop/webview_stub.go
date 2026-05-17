//go:build !desktop

package main

import (
	"context"
	"fmt"
	"os"
)

func runDesktopWebview(want bool, panelURL string, runtimeLossCh <-chan string, baseURL string, stopRoot context.CancelFunc, rootCtx context.Context) {
	if want {
		fmt.Fprintln(os.Stderr, "locus-desktop: desktop mode requires CGO and building with -tags desktop (try: make locus-desktop-build)")
		os.Exit(2)
	}
	<-rootCtx.Done()
}
