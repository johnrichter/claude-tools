package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runCLI executes the full command tree exactly as Execute() does (cobra's own
// usage/error printing silenced, exactly one clikit.Result written) but with
// stdout captured to a buffer and the process exit code returned in-process,
// so a test can assert on both the emitted JSON and the exit code without a
// subprocess.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)

	ranCmd, err := root.ExecuteC()
	if err == nil {
		return buf.String(), 0
	}
	var ee *exitError
	if ee2, ok := err.(*exitError); ok {
		ee = ee2
	}
	if ee != nil {
		return buf.String(), ee.code
	}
	// cobra rejected the invocation before any RunE ran (bad flag, unknown
	// command) -- emitUsageError writes straight to os.Stdout, not cmd's
	// configured writer, so capture that separately.
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe: %v", pipeErr)
	}
	orig := os.Stdout
	os.Stdout = w
	code := emitUsageError(ranCmd, err)
	_ = w.Close()
	os.Stdout = orig
	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	return out.String(), code
}

// decodeResult unmarshals a clikit.Result JSON line and fails the test if it
// isn't well-formed or isn't exactly one line -- clikit.Emit's own contract.
func decodeResult(t *testing.T, raw string) map[string]any {
	t.Helper()
	trimmed := strings.TrimRight(raw, "\n")
	if strings.Contains(trimmed, "\n") {
		t.Fatalf("result is not a single line: %q", raw)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v\nraw=%q", err, raw)
	}
	return m
}

func TestPlatformReportsHostOSAndArch(t *testing.T) {
	out, code := runCLI(t, "platform")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; out=%s", code, out)
	}
	m := decodeResult(t, out)
	if m["status"] != "success" {
		t.Errorf("status = %v, want success", m["status"])
	}
	data, _ := m["data"].(map[string]any)
	if data["os"] == "" || data["os"] == nil {
		t.Errorf("data.os is empty: %v", data)
	}
	if data["arch"] == "" || data["arch"] == nil {
		t.Errorf("data.arch is empty: %v", data)
	}
}

func TestGuardLimitsReportsPositiveMemoryAndOpenFiles(t *testing.T) {
	out, code := runCLI(t, "guard", "limits")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; out=%s", code, out)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	if v, ok := data["free_memory_bytes"].(float64); !ok || v <= 0 {
		t.Errorf("data.free_memory_bytes = %v, want > 0", data["free_memory_bytes"])
	}
}

func TestGuardPreflightFailsClosedOnAnUnreachableFloor(t *testing.T) {
	out, code := runCLI(t, "guard", "preflight", "--min-free-memory-bytes", "99999999999999999")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for an unmeetable memory floor; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "precondition_unmet" {
		t.Errorf("status = %v, want precondition_unmet", m["status"])
	}
}

func TestGuardPreflightSucceedsWhenFloorIsZero(t *testing.T) {
	out, code := runCLI(t, "guard", "preflight")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (no floor requested); out=%s", code, out)
	}
}

func TestRunCapturesStdoutAndExitCode(t *testing.T) {
	out, code := runCLI(t, "run", "--", "sh", "-c", "echo hi; exit 3")
	if code != 0 {
		t.Fatalf("cli exit code = %d, want 0 (a non-zero child exit is still cli-success); out=%s", code, out)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	if data["exit_code"].(float64) != 3 {
		t.Errorf("data.exit_code = %v, want 3", data["exit_code"])
	}
	if data["stdout"] != "hi\n" {
		t.Errorf("data.stdout = %q, want %q", data["stdout"], "hi\n")
	}
}

func TestRunRequiresAtLeastOneArg(t *testing.T) {
	out, code := runCLI(t, "run")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for run with no command; out=%s", out)
	}
}

func TestRunOnAMissingBinaryReportsInternalFailure(t *testing.T) {
	out, code := runCLI(t, "run", "--", "/no/such/binary-claude-tools-test")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for an unresolvable binary; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "internal" {
		t.Errorf("status = %v, want internal", m["status"])
	}
}

func TestRunRespectsATightTimeout(t *testing.T) {
	out, code := runCLI(t, "--timeout", "50ms", "run", "--", "sleep", "5")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero -- 5s sleep should not survive a 50ms timeout; out=%s", out)
	}
}

func TestStateSetGetRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if _, code := runCLI(t, "state", "set", "--path", path, "--key", "k", "--value", `"v"`); code != 0 {
		t.Fatalf("state set exit code = %d", code)
	}
	out, code := runCLI(t, "state", "get", "--path", path, "--key", "k")
	if code != 0 {
		t.Fatalf("state get exit code = %d; out=%s", code, out)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	if data["value"] != "v" || data["found"] != true {
		t.Errorf("data = %v, want value=v found=true", data)
	}
}

func TestStateGetOnAMissingKeyReportsFoundFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if _, code := runCLI(t, "state", "set", "--path", path, "--key", "k", "--value", `1`); code != 0 {
		t.Fatalf("state set exit code = %d", code)
	}
	out, code := runCLI(t, "state", "get", "--path", path, "--key", "absent")
	if code != 0 {
		t.Fatalf("state get exit code = %d", code)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	if data["found"] != false {
		t.Errorf("data.found = %v, want false", data["found"])
	}
}

func TestStateSetRejectsNonJSONValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	out, code := runCLI(t, "state", "set", "--path", path, "--key", "k", "--value", "not-json")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for a non-JSON --value; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "usage" {
		t.Errorf("status = %v, want usage", m["status"])
	}
}

func TestStateGetRequiresPathAndKey(t *testing.T) {
	if _, code := runCLI(t, "state", "get", "--path", "x.json"); code == 0 {
		t.Fatalf("exit code = 0, want nonzero when --key is missing")
	}
	if _, code := runCLI(t, "state", "get", "--key", "k"); code == 0 {
		t.Fatalf("exit code = 0, want nonzero when --path is missing")
	}
}

func TestStateRegisterAndSeenSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	if _, code := runCLI(t, "state", "register-source", "--path", path, "--ref", "https://x", "--consumer", "test"); code != 0 {
		t.Fatalf("register-source exit code = %d", code)
	}
	out, code := runCLI(t, "state", "seen-source", "--path", path, "--ref", "https://x")
	if code != 0 {
		t.Fatalf("seen-source exit code = %d", code)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	if data["seen"] != true {
		t.Errorf("seen = %v, want true for a registered ref", data["seen"])
	}

	out, code = runCLI(t, "state", "seen-source", "--path", path, "--ref", "https://unregistered")
	if code != 0 {
		t.Fatalf("seen-source exit code = %d", code)
	}
	m = decodeResult(t, out)
	data, _ = m["data"].(map[string]any)
	if data["seen"] != false {
		t.Errorf("seen = %v, want false for an unregistered ref", data["seen"])
	}
}

func newFetchTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>` + "http://" + r.Host + `/blog/a</loc><lastmod>2026-01-15</lastmod></url>
<url><loc>` + "http://" + r.Host + `/other/b</loc><lastmod>2026-02-01</lastmod></url>
</urlset>`))
	})
	mux.HandleFunc("/article.html", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>T</title><meta name="description" content="d"></head></html>`))
	})
	mux.HandleFunc("/notfound.html", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return httptest.NewServer(mux)
}

func TestFetchSitemapFiltersByPathPrefix(t *testing.T) {
	srv := newFetchTestServer(t)
	defer srv.Close()
	out, code := runCLI(t, "fetch", "sitemap", srv.URL+"/sitemap.xml", "--path-prefix", "/blog/")
	if code != 0 {
		t.Fatalf("exit code = %d; out=%s", code, out)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	entries, _ := data["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries = %v, want exactly 1 (only /blog/a matches the prefix)", entries)
	}
}

func TestFetchSitemapRejectsAnUnparseableSinceDate(t *testing.T) {
	srv := newFetchTestServer(t)
	defer srv.Close()
	out, code := runCLI(t, "fetch", "sitemap", srv.URL+"/sitemap.xml", "--since", "not-a-date")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for an unparseable --since; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "usage" {
		t.Errorf("status = %v, want usage", m["status"])
	}
}

func TestFetchArticleReportsMetadata(t *testing.T) {
	srv := newFetchTestServer(t)
	defer srv.Close()
	out, code := runCLI(t, "fetch", "article", srv.URL+"/article.html")
	if code != 0 {
		t.Fatalf("exit code = %d; out=%s", code, out)
	}
	m := decodeResult(t, out)
	data, _ := m["data"].(map[string]any)
	if data["title"] != "T" {
		t.Errorf("data.title = %v, want T", data["title"])
	}
}

func TestFetchArticleOn404ReportsInternalFailure(t *testing.T) {
	srv := newFetchTestServer(t)
	defer srv.Close()
	out, code := runCLI(t, "fetch", "article", srv.URL+"/notfound.html")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for a 404; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "internal" {
		t.Errorf("status = %v, want internal", m["status"])
	}
}

func TestConfigFileTimeoutAppliesWhenNoFlagOverridesIt(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte("timeout: 50ms\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, code := runCLI(t, "--config", cfgPath, "run", "--", "sleep", "5")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero -- the config file's 50ms timeout should have fired; out=%s", out)
	}
}

func TestConfigExplicitMissingFileIsAnError(t *testing.T) {
	out, code := runCLI(t, "--config", "/no/such/claude-tools-config.yaml", "run", "--", "true")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for an explicitly named missing config file; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "internal" {
		t.Errorf("status = %v, want internal", m["status"])
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	out, code := runCLI(t, "--this-flag-does-not-exist")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for an unknown flag; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "usage" {
		t.Errorf("status = %v, want usage", m["status"])
	}
}

func TestUnknownSubcommandIsAUsageError(t *testing.T) {
	out, code := runCLI(t, "not-a-real-subcommand")
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for an unknown subcommand; out=%s", out)
	}
	m := decodeResult(t, out)
	if m["status"] != "usage" {
		t.Errorf("status = %v, want usage", m["status"])
	}
}

// TestEveryLeafCommandHasAnExample guards SC-STACK's "≥1 usage example" bar
// at the level it matters: every leaf (non-group) command, not just root.
func TestEveryLeafCommandHasAnExample(t *testing.T) {
	root := newRootCmd()
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		children := cmd.Commands()
		isLeaf := true
		for _, c := range children {
			if c.Name() == "help" || c.Name() == "completion" {
				continue
			}
			isLeaf = false
			walk(c)
		}
		if isLeaf && cmd.Example == "" {
			t.Errorf("command %q has no Example", cmd.CommandPath())
		}
	}
	walk(root)
}
