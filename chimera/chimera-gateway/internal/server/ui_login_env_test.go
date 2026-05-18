package server

import (
	"github.com/lynn/porcelain/internal/naming"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestUILoginGET_autoLoginFromEnvToken(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	t.Setenv(naming.EnvGatewayTokenTarget, "gw-ui-secret")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(front.URL + "/ui/login")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("want redirect, got %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/logs" {
		t.Fatalf("Location: %q", loc)
	}
	if c := res.Header.Values("Set-Cookie"); len(c) == 0 || !strings.Contains(strings.Join(c, ";"), "chimera_ui_session=") {
		t.Fatalf("expected session Set-Cookie, got %v", c)
	}
}

func TestUILoginGET_autoLoginRespectsNextQuery(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	t.Setenv(naming.EnvGatewayTokenTarget, "gw-ui-secret")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = nil
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(front.URL + "/ui/login?next=/ui/desktop")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("want redirect, got %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/desktop" {
		t.Fatalf("Location: %q", loc)
	}
}

func TestUIDesktopRouteExistsWhenLogStoreDisabled(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	t.Setenv(naming.EnvGatewayTokenTarget, "gw-ui-secret")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = nil
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Get(front.URL + "/ui/login?next=/ui/desktop")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("want redirect, got %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/desktop" {
		t.Fatalf("Location: %q", loc)
	}

	res2, err := client.Get(front.URL + "/ui/desktop")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusFound {
		t.Fatalf("want auth redirect from desktop route, got %d", res2.StatusCode)
	}
	if loc := res2.Header.Get("Location"); !strings.Contains(loc, "/ui/login") {
		t.Fatalf("Location: %q expected login redirect", loc)
	}
}

func TestUILoginGET_badEnvTokenShowsLoginPage(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	t.Setenv(naming.EnvGatewayTokenTarget, "not-a-valid-token")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	res, err := http.Get(front.URL + "/ui/login")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "Chimera admin") {
		head := s
		if len(head) > 200 {
			head = head[:200]
		}
		t.Fatalf("expected login HTML, got %q", head)
	}
}

func TestUILoginGET_openRedirectQuerySanitized(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	t.Setenv(naming.EnvGatewayTokenTarget, "gw-ui-secret")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	q := url.Values{}
	q.Set("next", "//evil.example/phish")
	res, err := client.Get(front.URL + "/ui/login?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("want redirect, got %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/logs" {
		t.Fatalf("Location: %q", loc)
	}
}

func TestUILoginGET_envTokenAllowsAuthenticatedAPI(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	t.Setenv(naming.EnvGatewayTokenTarget, "gw-ui-secret")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	loginRes, err := client.Get(front.URL + "/ui/login")
	if err != nil {
		t.Fatal(err)
	}
	_ = loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusFound {
		t.Fatalf("login GET %d", loginRes.StatusCode)
	}

	stateRes, err := client.Get(front.URL + "/api/ui/state")
	if err != nil {
		t.Fatal(err)
	}
	defer stateRes.Body.Close()
	if stateRes.StatusCode != http.StatusOK {
		t.Fatalf("state %d", stateRes.StatusCode)
	}
}

func TestUILoginGET_unsetEnvDoesNotRedirect(t *testing.T) {
	t.Setenv(naming.EnvGatewayTokenTarget, "")
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	res, err := http.Get(front.URL + "/ui/login")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}
