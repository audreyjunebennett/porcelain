//go:build desktop

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gen2brain/dlgs"
	"github.com/lynn/porcelain/internal/platform"
	webview "github.com/webview/webview_go"
)

func runDesktopWebview(want bool, panelURL string, runtimeLossCh <-chan string, baseURL string, stopRoot context.CancelFunc, rootCtx context.Context) {
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
	if runtimeLossCh != nil {
		go func() {
			reason, ok := <-runtimeLossCh
			if !ok || strings.TrimSpace(reason) == "" {
				return
			}
			lossURL := buildUnreachableURL(baseURL, "Supervisor connection lost during runtime: "+reason, false)
			w.Dispatch(func() {
				w.Navigate(lossURL)
			})
		}()
	}

	if err := w.Bind("chimeraPickFolder", func(startDir string) (string, error) {
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
		fmt.Fprintf(os.Stderr, "locus-desktop: chimeraPickFolder bind: %v\n", err)
	}

	if err := w.Bind("chimeraOpenExternalURL", func(raw string) (string, error) {
		return "", platform.OpenURLInBrowser(raw)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "locus-desktop: chimeraOpenExternalURL bind: %v\n", err)
	}
	if err := w.Bind("chimeraRevealProjectPath", func(rel string) (string, error) {
		return "", platform.RevealProjectPath(rel)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "locus-desktop: chimeraRevealProjectPath bind: %v\n", err)
	}

	w.SetTitle("Locus")
	w.SetSize(1024, 720, webview.HintNone)
	w.Navigate(panelURL)
	w.Run()
	stopRoot()
}
