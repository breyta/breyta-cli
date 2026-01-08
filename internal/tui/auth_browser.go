package tui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type browserLoginResult struct {
	Token        string
	RefreshToken string
	ExpiresIn    string
}

func browserLogin(ctx context.Context, apiBaseURL string, out io.Writer) (browserLoginResult, error) {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if apiBaseURL == "" {
		return browserLoginResult{}, errors.New("missing api base url")
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return browserLoginResult{}, err
	}
	defer l.Close()

	st := make([]byte, 32)
	if _, err := rand.Read(st); err != nil {
		return browserLoginResult{}, err
	}
	state := base64.RawURLEncoding.EncodeToString(st)

	addr := l.Addr().String()
	callbackURL := "http://" + addr + "/callback"

	tokenCh := make(chan browserLoginResult, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		tok := strings.TrimSpace(q.Get("token"))
		if tok == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}
		refresh := strings.TrimSpace(q.Get("refresh_token"))
		expiresIn := strings.TrimSpace(q.Get("expires_in"))
		_, _ = io.WriteString(w, "<html><body>Login complete. You can close this tab.</body></html>")
		select {
		case tokenCh <- browserLoginResult{Token: tok, RefreshToken: refresh, ExpiresIn: expiresIn}:
		default:
		}
	})

	go func() {
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	authURL := apiBaseURL + "/cli/auth?redirect_uri=" + url.QueryEscape(callbackURL) + "&state=" + url.QueryEscape(state)
	if out != nil {
		fmt.Fprintln(out, "Opening browser for login:")
		fmt.Fprintln(out, authURL)
	}
	if err := openBrowser(authURL); err != nil && out != nil {
		fmt.Fprintln(out, "Could not open browser automatically; open the URL above manually.")
	}

	timeout := 2 * time.Minute
	select {
	case res := <-tokenCh:
		_ = srv.Shutdown(context.Background())
		return res, nil
	case err := <-errCh:
		_ = srv.Shutdown(context.Background())
		return browserLoginResult{}, err
	case <-time.After(timeout):
		_ = srv.Shutdown(context.Background())
		return browserLoginResult{}, errors.New("login timed out (no callback received)")
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return browserLoginResult{}, ctx.Err()
	}
}

func openBrowser(u string) error {
	u = strings.TrimSpace(u)
	if u == "" {
		return errors.New("missing url")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}

