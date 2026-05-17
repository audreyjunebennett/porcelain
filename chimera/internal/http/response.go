package internalhttp

import (
	"io"
	"strings"
)

// ReadAndCloseLimited reads up to limit bytes from rc and closes it.
func ReadAndCloseLimited(rc io.ReadCloser, limit int64) ([]byte, error) {
	if rc == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(rc, limit))
	closeErr := rc.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return body, nil
}

// BearerAuthValue builds an Authorization Bearer header value.
func BearerAuthValue(token string) string {
	t := strings.TrimSpace(token)
	if t == "" {
		return ""
	}
	return "Bearer " + t
}
