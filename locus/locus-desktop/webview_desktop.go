package main

import (
	"strings"

	"github.com/gen2brain/dlgs"
	"github.com/lynn/porcelain/internal/locus"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/app"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/page"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/platform"
	webview "github.com/webview/webview_go"
)

func runDesktopWebview(req app.UIRequest) {
	w := webview.New(false)
	defer w.Destroy()

	go func() {
		<-req.RootCtx.Done()
		w.Terminate()
	}()

	if req.RuntimeLossCh != nil {
		go func() {
			reason, ok := <-req.RuntimeLossCh
			if !ok || strings.TrimSpace(reason) == "" {
				return
			}
			lossURL := page.RuntimeLossDataURL(req.BaseURL, "Supervisor connection lost during runtime: "+reason)
			w.Dispatch(func() {
				w.Navigate(lossURL)
			})
		}()
	}

	if err := w.Bind(locus.BridgePickFolder, func(startDir string) (string, error) {
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
		locus.Logf("%s bind: %v\n", locus.BridgePickFolder, err)
	}

	if err := w.Bind(locus.BridgeOpenExternalURL, func(raw string) (string, error) {
		return "", platform.OpenURLInBrowser(raw)
	}); err != nil {
		locus.Logf("%s bind: %v\n", locus.BridgeOpenExternalURL, err)
	}
	if err := w.Bind(locus.BridgeRevealProjectPath, func(rel string) (string, error) {
		return "", platform.RevealProjectPath(rel)
	}); err != nil {
		locus.Logf("%s bind: %v\n", locus.BridgeRevealProjectPath, err)
	}

	w.SetTitle(locus.WindowTitle)
	w.SetSize(1024, 720, webview.HintNone)
	w.Navigate(req.PanelURL)
	w.Run()
	req.StopRoot()
}
