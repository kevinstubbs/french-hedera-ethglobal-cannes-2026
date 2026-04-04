package middleware

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/x402test"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found from test working directory")
	return ""
}

func parseTSRouteKeys(src string) []string {
	start := strings.Index(src, "PIPELINE_X402_ROUTE_KEYS")
	if start < 0 {
		return nil
	}
	sub := src[start:]
	i := strings.Index(sub, "[")
	j := strings.Index(sub, "] as const")
	if i < 0 || j < 0 || j <= i {
		return nil
	}
	block := sub[i:j]
	re := regexp.MustCompile(`"(POST|PUT) /v1[^"]+"`)
	raw := re.FindAllString(block, -1)
	keys := make([]string, 0, len(raw))
	for _, m := range raw {
		keys = append(keys, strings.Trim(m, `"`))
	}
	return keys
}

func TestPipelineRoutesMatchTypeScriptMirror(t *testing.T) {
	root := findRepoRoot(t)
	tsPath := filepath.Join(root, "agents", "trading-signal", "src", "pipelineX402Routes.ts")
	b, err := os.ReadFile(tsPath)
	if err != nil {
		t.Fatalf("read TS mirror: %v", err)
	}
	tsKeys := parseTSRouteKeys(string(b))
	sort.Strings(tsKeys)

	rc := PipelineRoutes(x402test.TestX402Config())
	var goKeys []string
	for k := range rc {
		goKeys = append(goKeys, k)
	}
	sort.Strings(goKeys)

	if len(goKeys) != len(tsKeys) {
		t.Fatalf("route count mismatch: go=%d ts=%d\ngo=%q\nts=%q", len(goKeys), len(tsKeys), goKeys, tsKeys)
	}
	for i := range goKeys {
		if goKeys[i] != tsKeys[i] {
			t.Fatalf("route mismatch at %d: go %q vs ts %q", i, goKeys[i], tsKeys[i])
		}
	}
}
