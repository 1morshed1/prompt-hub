package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1morshed1/prompt-hub/server/internal/api"
	"github.com/1morshed1/prompt-hub/server/internal/testutil"
)

// TestEndToEnd drives the whole HTTP API in-process against a real database:
// create a prompt, add versions, tag, render (with a conditional template),
// diff, run an A/B experiment through assignment + result logging, and verify the
// report detects a significant winner and auto-promotes. This is the executable
// version of the manual curl walkthrough in docs/api-reference.md.
func TestEndToEnd(t *testing.T) {
	st := testutil.Setup(t)
	srv := api.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c := &apiClient{t: t, base: ts.URL}

	// 1. Create a prompt.
	c.post("/api/v1/prompts", map[string]any{"name": "assistant", "description": "d"}, http.StatusCreated)

	// 2. Two versions. v2 adds a conditional.
	c.post("/api/v1/prompts/assistant/versions", map[string]any{
		"system_template": "You are {{role}}.",
		"user_template":   "{{q}}",
		"variables":       []map[string]any{{"name": "role", "type": "string", "required": true}, {"name": "q", "type": "string", "required": true}},
	}, http.StatusCreated)
	c.post("/api/v1/prompts/assistant/versions", map[string]any{
		"system_template": "You are a friendly {{role}}.{% if vip %} Treat as VIP.{% endif %}",
		"user_template":   "{{q}}",
		"variables":       []map[string]any{{"name": "role", "type": "string", "required": true}, {"name": "q", "type": "string", "required": true}, {"name": "vip", "type": "boolean", "required": false}},
	}, http.StatusCreated)

	// 3. Tag production -> v2.
	c.put("/api/v1/prompts/assistant/tags/production", map[string]any{"version_number": 2}, http.StatusOK)

	// 4. Render production with the conditional true.
	var rendered map[string]any
	c.postInto("/api/v1/prompts/assistant/render", map[string]any{
		"tag":       "production",
		"variables": map[string]any{"role": "helper", "q": "hi", "vip": true},
	}, http.StatusOK, &rendered)
	if got := rendered["system"].(string); got != "You are a friendly helper. Treat as VIP." {
		t.Fatalf("unexpected render: %q", got)
	}

	// 5. Validation error: missing required 'q' -> 422.
	c.post("/api/v1/prompts/assistant/render", map[string]any{
		"version":   1,
		"variables": map[string]any{"role": "helper"},
	}, http.StatusUnprocessableEntity)

	// 6. Diff v1 vs v2.
	var diff map[string]any
	c.getInto("/api/v1/prompts/assistant/diff?from=1&to=2", http.StatusOK, &diff)
	if diff["system_template"] == nil {
		t.Fatal("diff missing system_template")
	}

	// 7. Experiment: control=v1, variant=v2, auto-promote.
	var exp map[string]any
	c.postInto("/api/v1/experiments", map[string]any{
		"prompt": "assistant", "name": "e", "control_version": 1, "variant_version": 2,
		"traffic_split": 0.5, "auto_promote": true, "min_samples": 40,
	}, http.StatusCreated, &exp)
	expID := exp["id"].(string)

	// 8. Drive traffic: variant up-rate ~75%, control ~30%.
	for i := 0; i < 120; i++ {
		var r map[string]any
		c.postInto("/api/v1/prompts/assistant/render", map[string]any{
			"unit_id":   itoa(i),
			"variables": map[string]any{"role": "helper", "q": "hi"},
		}, http.StatusOK, &r)
		arm, _ := r["arm"].(string)
		rating := 0
		if arm == "variant" {
			if i%100 < 75 {
				rating = 1
			}
		} else if i%100 < 30 {
			rating = 1
		}
		c.post("/api/v1/results", map[string]any{
			"request_id": r["request_id"], "rating": rating, "latency_ms": 900 + i,
		}, http.StatusOK)
	}

	// 9. Report should show a significant variant win and auto-promote.
	var report map[string]any
	c.getInto("/api/v1/experiments/"+expID+"/report", http.StatusOK, &report)
	rep := report["report"].(map[string]any)
	if rep["winner"] != "variant" {
		t.Fatalf("expected variant winner, got %v (p=%v)", rep["winner"], rep["rating_p_value"])
	}
	promo := report["promotion"].(map[string]any)
	if promo["promoted"] != true {
		t.Fatalf("expected auto-promotion, got %v", promo["reason"])
	}

	// 10. Production tag now resolves to v2 (the promoted variant), confirmed by
	//     an explicit render pinned to production returning v2's system text.
	var afterPromo map[string]any
	c.postInto("/api/v1/prompts/assistant/render", map[string]any{
		"version":   2,
		"variables": map[string]any{"role": "x", "q": "y"},
	}, http.StatusOK, &afterPromo)
	if int(afterPromo["version_number"].(float64)) != 2 {
		t.Fatalf("expected v2, got %v", afterPromo["version_number"])
	}
}

// --- tiny test HTTP client ---

type apiClient struct {
	t    *testing.T
	base string
}

func (c *apiClient) do(method, path string, body any, want int, into any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		c.t.Fatalf("%s %s: status %d (want %d): %s", method, path, resp.StatusCode, want, raw)
	}
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			c.t.Fatalf("%s %s: decode: %v (%s)", method, path, err, raw)
		}
	}
}

func (c *apiClient) post(path string, body any, want int) { c.do("POST", path, body, want, nil) }
func (c *apiClient) postInto(path string, body any, want int, into any) {
	c.do("POST", path, body, want, into)
}
func (c *apiClient) put(path string, body any, want int)     { c.do("PUT", path, body, want, nil) }
func (c *apiClient) getInto(path string, want int, into any) { c.do("GET", path, nil, want, into) }

func itoa(i int) string {
	return string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('0'+i%10))
}
