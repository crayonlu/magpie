package gateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// backendLeads is a turn as the ChatGPT backend answers it: it says what it
// knows of the account first, in the two events only Codex has a name for
// (codex.rate_limits with its windows, codex.response.metadata with the
// model list's tag), then the reply itself.
var backendLeads = sse(
	`event: codex.rate_limits`+"\n"+`data: {"type":"codex.rate_limits","plan_type":"plus","rate_limits":{"allowed":true,"limit_reached":false,"primary":{"used_percent":3,"window_minutes":300,"reset_after_seconds":100}}}`,
	`event: codex.response.metadata`+"\n"+`data: {"type":"codex.response.metadata","headers":{"x-models-etag":"W\"9e0\""}}`,
	`event: response.created`+"\n"+`data: {"type":"response.created","response":{"id":"r1","model":"gpt-5.5"}}`,
	`event: response.output_text.delta`+"\n"+`data: {"type":"response.output_text.delta","delta":"a cat"}`,
	`event: response.completed`+"\n"+`data: {"type":"response.completed","response":{"id":"r1","status":"completed","usage":{"input_tokens":9,"output_tokens":2}}}`)

// leadSignIn puts one ChatGPT sign-in in a temp home and points the
// backend at f, as the ChatGPT answers come from there.
func leadSignIn(t *testing.T, f http.Handler) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	claims := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return "h." + base64.RawURLEncoding.EncodeToString(b) + ".s"
	}
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	auth := fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"id_token":%q,"access_token":%q,"refresh_token":"r","account_id":"acct-1"}}`,
		claims(map[string]any{"email": "me@example.com"}),
		claims(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}))
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	up := httptest.NewServer(f)
	t.Cleanup(up.Close)
	old := provider.CodexBase
	provider.CodexBase = up.URL + "/backend-api/codex"
	t.Cleanup(func() { provider.CodexBase = old })
}

// leadTurn posts a Responses turn with some client's User-Agent.
func leadTurn(t *testing.T, body, ua string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer magpie")
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	New().Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// A client that isn't Codex gets a ChatGPT account's reply with the
// backend's own events gone: Grok Build reads the stream through a typed
// event enum, and an event type it has no name for ends the turn with
// "unknown variant codex.rate_limits" before any of the reply is seen.
// Codex's own request still gets them, which is where it reads its windows.
func TestCodexLeadEventsStayWithCodex(t *testing.T) {
	const body = `{"model":"codex/gpt-5.5","stream":true,"input":"what is this?"}`
	for _, who := range []struct {
		ua   string
		want bool // the account's events are for this client
		says string
	}{
		{"", false, "a client with no User-Agent"},
		{"xai-grok-shell/1.0.45", false, "Grok Build"},
		{"codex_cli_rs/0.166.0", true, "Codex CLI"},
	} {
		f := &fake{t: t, ctype: "none", reply: backendLeads}
		leadSignIn(t, f)
		code, got := leadTurn(t, body, who.ua)
		if code != 200 || !strings.Contains(got, `"delta":"a cat"`) || !strings.Contains(got, "response.completed") {
			t.Fatalf("%s: %d %s", who.says, code, got)
		}
		for _, ev := range []string{"codex.rate_limits", "codex.response.metadata"} {
			if has := strings.Contains(got, ev); has != who.want {
				t.Errorf("%s: event %s there=%v, want %v: %s", who.says, ev, has, who.want, got)
			}
		}
		if f.path != "/backend-api/codex/responses" {
			t.Errorf("%s: asked %s", who.says, f.path)
		}
	}
}
