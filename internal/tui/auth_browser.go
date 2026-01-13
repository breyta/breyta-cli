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
	"strings"
	"sync"
	"time"

	"github.com/breyta/breyta-cli/internal/browseropen"
)

type browserLoginResult struct {
	Token        string
	RefreshToken string
	ExpiresIn    string
}

type browserLoginSession struct {
	AuthURL string

	tokenCh chan browserLoginResult
	errCh   chan error
	srv     *http.Server
	l       net.Listener

	cleanupOnce sync.Once
}

func (s *browserLoginSession) cleanup() {
	s.cleanupOnce.Do(func() {
		if s.srv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.srv.Shutdown(ctx)
			cancel()
		}
		if s.l != nil {
			_ = s.l.Close()
		}
	})
}

func (s *browserLoginSession) Wait(ctx context.Context) (browserLoginResult, error) {
	defer s.cleanup()

	timeout := 2 * time.Minute
	select {
	case res := <-s.tokenCh:
		return res, nil
	case err := <-s.errCh:
		return browserLoginResult{}, err
	case <-time.After(timeout):
		return browserLoginResult{}, errors.New("login timed out (no callback received)")
	case <-ctx.Done():
		return browserLoginResult{}, ctx.Err()
	}
}

func startBrowserLogin(apiBaseURL string) (*browserLoginSession, error, error) {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if apiBaseURL == "" {
		return nil, nil, errors.New("missing api base url")
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}

	st := make([]byte, 32)
	if _, err := rand.Read(st); err != nil {
		_ = l.Close()
		return nil, nil, err
	}
	state := base64.RawURLEncoding.EncodeToString(st)

	addr := l.Addr().String()
	callbackURL := "http://" + addr + "/callback"

	sess := &browserLoginSession{
		tokenCh: make(chan browserLoginResult, 1),
		errCh:   make(chan error, 1),
		l:       l,
	}

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	sess.srv = srv

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
		case sess.tokenCh <- browserLoginResult{Token: tok, RefreshToken: refresh, ExpiresIn: expiresIn}:
		default:
		}
	})

	go func() {
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sess.errCh <- err
		}
	}()

	sess.AuthURL = apiBaseURL + "/cli/auth?redirect_uri=" + url.QueryEscape(callbackURL) + "&state=" + url.QueryEscape(state)
	openErr := openBrowser(sess.AuthURL)
	return sess, openErr, nil
}

func browserLogin(ctx context.Context, apiBaseURL string, out io.Writer) (browserLoginResult, error) {
	sess, openErr, err := startBrowserLogin(apiBaseURL)
	if err != nil {
		return browserLoginResult{}, err
	}
	authURL := sess.AuthURL
	if out != nil {
		fmt.Fprintln(out, "Opening browser for login:")
		fmt.Fprintln(out, authURL)
	}
	if openErr != nil && out != nil {
		fmt.Fprintln(out, "Could not open browser automatically; open the URL above manually.")
	}
	return sess.Wait(ctx)
}

func openBrowser(u string) error {
	return browseropen.Open(u)
}
