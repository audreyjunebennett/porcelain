//go:build desktop

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gen2brain/dlgs"
	"github.com/lynn/claudia-gateway/internal/platform"
	webview "github.com/webview/webview_go"
)

func runDesktopWebview(want bool, panelURL string, stopRoot context.CancelFunc, rootCtx context.Context) {
	if !want {
		<-rootCtx.Done()
		return
	}
	w := webview.New(false)
	defer w.Destroy()

	go func() {
		<-rootCtx.Done()
		w.Terminate()
	}()

	// Optional startDir is passed to the platform folder dialog when supported; empty is fine.
	if err := w.Bind("claudiaPickFolder", func(startDir string) (string, error) {
		startDir = strings.TrimSpace(startDir)
		path, ok, err := dlgs.File("Select folder to index", startDir, true)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", nil
		}
		return path, nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "claudia desktop: claudiaPickFolder bind: %v\n", err)
	}

	if err := w.Bind("claudiaOpenExternalURL", func(raw string) (string, error) {
		return "", platform.OpenURLInBrowser(raw)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "claudia desktop: claudiaOpenExternalURL bind: %v\n", err)
	}
	if err := w.Bind("claudiaRevealProjectPath", func(rel string) (string, error) {
		return "", platform.RevealProjectPath(rel)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "claudia desktop: claudiaRevealProjectPath bind: %v\n", err)
	}

	w.SetTitle("Claudia")
	w.SetSize(1024, 720, webview.HintNone)
	w.Navigate(panelURL)
	w.Dispatch(func() {
		setWebviewWindowIcon(w)
	})
	w.Run()
	stopRoot()
}
