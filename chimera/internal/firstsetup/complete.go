package firstsetup

import (
	"fmt"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/envfile"
	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/internal/naming"
)

const (
	AccessOpen          = "open"
	AccessAuthenticated = "authenticated"
	LoginAutomatic      = "automatic"
	LoginManual         = "manual"
	localOnlyListenHost = "127.0.0.1"
)

// OpenModeBuiltInSecret is the fixed local-only gateway credential for open mode.
const OpenModeBuiltInSecret = "chimera-local-open"

// Request is the first-run setup choice from the operator UI.
type Request struct {
	AccessMode string
	LoginMode  string
}

// Options configures Complete.
type Options struct {
	TokensPath  string
	GatewayPath string
	DotenvPath  string
	EnvKey      string
}

// Result is returned after a successful first-run setup.
type Result struct {
	AccessMode        string
	LoginMode         string
	Token             string
	TokenShown        bool
	TokenSavedToEnv   bool
	EnvSkipReason     string
	ListenHostPatched bool
}

// Complete applies the operator's first-run choices: credentials, optional listen binding, optional dotenv.
func Complete(req Request, opts Options) (Result, error) {
	access := strings.ToLower(strings.TrimSpace(req.AccessMode))
	login := strings.ToLower(strings.TrimSpace(req.LoginMode))

	switch access {
	case AccessOpen:
		login = LoginAutomatic
	case AccessAuthenticated:
		if login != LoginAutomatic && login != LoginManual {
			return Result{}, fmt.Errorf("login_mode must be automatic or manual for authenticated mode")
		}
	default:
		return Result{}, fmt.Errorf("access_mode must be open or authenticated")
	}

	tokensPath := strings.TrimSpace(opts.TokensPath)
	if tokensPath == "" {
		return Result{}, fmt.Errorf("tokens path is required")
	}
	if !tokens.IsBootstrapMode(tokensPath) {
		return Result{}, fmt.Errorf("setup already completed")
	}

	var (
		secret string
		label  string
		tenant string
		res    Result
	)
	switch access {
	case AccessOpen:
		secret = OpenModeBuiltInSecret
		label = "local-open"
		tenant = "local"
		res.ListenHostPatched = true
	case AccessAuthenticated:
		var err error
		secret, err = tokens.GenerateGatewayToken()
		if err != nil {
			return Result{}, err
		}
		label = "default"
		tenant = tokens.TenantIDFromLabel(label)
	}

	if err := tokens.WriteInitialCredential(tokensPath, secret, tenant, label); err != nil {
		return Result{}, fmt.Errorf("write api-keys: %w", err)
	}

	gatewayPath := strings.TrimSpace(opts.GatewayPath)
	if access == AccessOpen && gatewayPath != "" {
		if err := config.WriteGatewayListenHost(gatewayPath, localOnlyListenHost); err != nil {
			return Result{}, fmt.Errorf("write gateway listen_host: %w", err)
		}
	}

	envKey := strings.TrimSpace(opts.EnvKey)
	if envKey == "" {
		envKey = naming.EnvGatewayTokenTarget
	}
	dotenvPath := strings.TrimSpace(opts.DotenvPath)
	if dotenvPath == "" {
		dotenvPath = ".env"
	}

	saveEnv := access == AccessOpen || login == LoginAutomatic
	if saveEnv {
		upsert, err := envfile.UpsertIfAbsent(dotenvPath, envKey, secret)
		if err != nil {
			return Result{}, fmt.Errorf("write dotenv: %w", err)
		}
		res.TokenSavedToEnv = upsert.Written
		if !upsert.Written {
			res.EnvSkipReason = upsert.Reason
		}
	} else {
		res.EnvSkipReason = "manual_login"
	}

	res.AccessMode = access
	res.LoginMode = login
	if access == AccessAuthenticated && login == LoginManual {
		res.Token = secret
		res.TokenShown = true
	}

	return res, nil
}
