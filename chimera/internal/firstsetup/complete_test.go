package firstsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/internal/naming"
)

func TestComplete_openMode(t *testing.T) {
	dir := t.TempDir()
	tokensPath := filepath.Join(dir, naming.APIKeysFileTarget)
	gatewayPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	dotenvPath := filepath.Join(dir, ".env")
	writeGatewayYAML(t, gatewayPath, "0.0.0.0")

	res, err := Complete(Request{AccessMode: AccessOpen}, Options{
		TokensPath:  tokensPath,
		GatewayPath: gatewayPath,
		DotenvPath:  dotenvPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TokenSavedToEnv {
		t.Fatal("expected token saved to env for open mode")
	}
	if !res.ListenHostPatched {
		t.Fatal("expected listen host patched")
	}
	if res.TokenShown || res.Token != "" {
		t.Fatalf("open mode should not expose token: %+v", res)
	}
	if tokens.IsBootstrapMode(tokensPath) {
		t.Fatal("expected credentials written")
	}
	meta, err := tokens.ListTokenMeta(tokensPath)
	if err != nil || len(meta) != 1 || meta[0].Token != OpenModeBuiltInSecret {
		t.Fatalf("meta: %+v err=%v", meta, err)
	}
	rawGW, _ := os.ReadFile(gatewayPath)
	if !strings.Contains(string(rawGW), `listen_host: "127.0.0.1"`) {
		t.Fatalf("gateway: %s", rawGW)
	}
	rawEnv, _ := os.ReadFile(dotenvPath)
	if !strings.Contains(string(rawEnv), OpenModeBuiltInSecret) {
		t.Fatalf("env: %s", rawEnv)
	}
}

func TestComplete_authenticatedManual(t *testing.T) {
	dir := t.TempDir()
	tokensPath := filepath.Join(dir, naming.APIKeysFileTarget)
	gatewayPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	dotenvPath := filepath.Join(dir, ".env")
	writeGatewayYAML(t, gatewayPath, "0.0.0.0")

	res, err := Complete(Request{
		AccessMode: AccessAuthenticated,
		LoginMode:  LoginManual,
	}, Options{
		TokensPath:  tokensPath,
		GatewayPath: gatewayPath,
		DotenvPath:  dotenvPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TokenShown || len(res.Token) < 16 {
		t.Fatalf("expected token shown: %+v", res)
	}
	if res.TokenSavedToEnv || res.EnvSkipReason != "manual_login" {
		t.Fatalf("manual should skip env: %+v", res)
	}
	if _, err := os.Stat(dotenvPath); err == nil {
		t.Fatal("expected no dotenv for manual login")
	}
	rawGW, _ := os.ReadFile(gatewayPath)
	if strings.Contains(string(rawGW), `listen_host: "127.0.0.1"`) {
		t.Fatal("authenticated mode should not force localhost listen")
	}
}

func TestComplete_authenticatedAutomatic(t *testing.T) {
	dir := t.TempDir()
	tokensPath := filepath.Join(dir, naming.APIKeysFileTarget)
	dotenvPath := filepath.Join(dir, ".env")

	res, err := Complete(Request{
		AccessMode: AccessAuthenticated,
		LoginMode:  LoginAutomatic,
	}, Options{
		TokensPath: tokensPath,
		DotenvPath: dotenvPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TokenSavedToEnv || res.TokenShown {
		t.Fatalf("automatic: %+v", res)
	}
	rawEnv, _ := os.ReadFile(dotenvPath)
	if len(rawEnv) == 0 {
		t.Fatal("expected dotenv written")
	}
}

func writeGatewayYAML(t *testing.T, path, listenHost string) {
	t.Helper()
	body := "gateway:\n  semver: \"0.1.0\"\n  listen_port: 3000\n  listen_host: \"" + listenHost + "\"\n" +
		"upstream:\n  base_url: \"http://127.0.0.1:8080\"\n  api_key_env: \"CHIMERA_BROKER_API_KEY\"\n" +
		"paths:\n  api_keys: \"./api-keys.yaml\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
