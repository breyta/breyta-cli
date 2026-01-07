package cli_test

import (
        "bytes"
        "encoding/json"
        "net/http"
        "net/http/httptest"
        "strings"
        "testing"

        "breyta-cli/internal/cli"
)

func runCLIArgsWithIn(t *testing.T, stdin string, args ...string) (string, string, error) {
        t.Helper()
        cmd := cli.NewRootCmd()
        out := new(bytes.Buffer)
        errOut := new(bytes.Buffer)
        cmd.SetOut(out)
        cmd.SetErr(errOut)
        cmd.SetIn(strings.NewReader(stdin))
        cmd.SetArgs(args)
        err := cmd.Execute()
        return out.String(), errOut.String(), err
}

func TestAuthLogin_PrintsExportLine(t *testing.T) {
        var got map[string]any
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.URL.Path != "/api/auth/token" {
                        http.NotFound(w, r)
                        return
                }
                if r.Method != http.MethodPost {
                        w.WriteHeader(http.StatusMethodNotAllowed)
                        return
                }
                _ = json.NewDecoder(r.Body).Decode(&got)
                _ = json.NewEncoder(w).Encode(map[string]any{
                        "success":   true,
                        "token":     "id-token-123",
                        "uid":       "uid-123",
                        "expiresIn": 3600,
                })
        }))
        defer srv.Close()

        stdout, _, err := runCLIArgs(t,
                "--api", srv.URL,
                "--workspace", "ws-acme",
                "auth", "login",
                "--email", "a@b.com",
                "--password", "pw",
                "--print", "export",
        )
        if err != nil {
                t.Fatalf("auth login failed: %v\n%s", err, stdout)
        }
        if strings.TrimSpace(stdout) != "export BREYTA_TOKEN='id-token-123'" {
                t.Fatalf("unexpected stdout:\n%s", stdout)
        }
        if got["email"] != "a@b.com" || got["password"] != "pw" {
                t.Fatalf("unexpected payload: %+v", got)
        }
}

func TestAuthLogin_PasswordStdin_PrintsToken(t *testing.T) {
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.URL.Path != "/api/auth/token" {
                        http.NotFound(w, r)
                        return
                }
                _ = json.NewEncoder(w).Encode(map[string]any{
                        "success": true,
                        "token":   "id-token-xyz",
                })
        }))
        defer srv.Close()

        stdout, _, err := runCLIArgsWithIn(t, "pw-from-stdin\n",
                "--api", srv.URL,
                "auth", "login",
                "--email", "a@b.com",
                "--password-stdin",
                "--print", "token",
        )
        if err != nil {
                t.Fatalf("auth login failed: %v\n%s", err, stdout)
        }
        if strings.TrimSpace(stdout) != "id-token-xyz" {
                t.Fatalf("unexpected stdout:\n%s", stdout)
        }
}

func TestAuthWhoami_CallsVerify(t *testing.T) {
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.URL.Path != "/api/auth/verify" {
                        http.NotFound(w, r)
                        return
                }
                if got := r.Header.Get("Authorization"); got != "Bearer tok" {
                        w.WriteHeader(401)
                        _ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing token"})
                        return
                }
                _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "user": map[string]any{"id": "uid-1"}})
        }))
        defer srv.Close()

        stdout, _, err := runCLIArgs(t,
                "--api", srv.URL,
                "--token", "tok",
                "auth", "whoami",
                "--pretty",
        )
        if err != nil {
                t.Fatalf("auth whoami failed: %v\n%s", err, stdout)
        }
        if !strings.Contains(stdout, `"success": true`) && !strings.Contains(stdout, `"success":true`) {
                t.Fatalf("expected verify payload in output:\n%s", stdout)
        }
}
