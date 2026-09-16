//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/testutil"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/redisx"
)

// End-to-end tests drive the fully wired application (real MySQL in a
// container, in-process Redis) through HTTP, exactly as a client would.

var application *App

func TestMain(m *testing.M) {
	ctx := context.Background()

	mysql, err := testutil.StartMySQL(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup:", err)
		os.Exit(1)
	}
	mr, err := miniredis.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "miniredis:", err)
		os.Exit(1)
	}

	cfg := testConfig(mysql, mr.Addr())
	application, err = New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = application.shutdown()
	mr.Close()
	mysql.Close()
	os.Exit(code)
}

func testConfig(mysql *testutil.MySQL, redisAddr string) *config.Config {
	return &config.Config{
		App: config.AppConfig{Env: config.EnvTest, Name: "task-management", Version: "e2e"},
		Log: logger.Config{Level: "error", Format: "json", Output: "stderr"},
		HTTP: config.HTTPConfig{
			Port: 0, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: time.Minute,
			ShutdownTimeout: 5 * time.Second, RequestTimeout: 10 * time.Second, BodyLimitBytes: 1 << 20,
			AllowedOrigins: []string{"*"}, EnableMetrics: true,
		},
		DB:      mysql.Config,
		Redis:   redisx.Config{Addr: redisAddr, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, PoolSize: 10},
		Breaker: config.BreakerConfig{Enabled: true, ConsecutiveFailures: 5, OpenTimeout: time.Second},
		Auth: config.AuthConfig{
			Secret: "e2e-secret-e2e-secret-e2e-secret-e2e", Issuer: "e2e",
			AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour, BcryptCost: 10, CookieSameSite: "Lax",
		},
		Cache:     config.CacheConfig{TaskListTTL: 5 * time.Minute},
		RateLimit: config.RateLimitConfig{Enabled: false},
	}
}

// ---------- tiny HTTP client ----------

type client struct {
	t     *testing.T
	token string
}

type response struct {
	Status int
	Body   map[string]any
	List   []any
	Header http.Header
}

func (c client) do(method, path string, body any) response {
	c.t.Helper()

	var rdr io.Reader = http.NoBody
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, "/api/v1"+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := application.server.App().Test(req, 10_000)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	r := response{Status: resp.StatusCode, Header: resp.Header}
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &r.List)
	} else {
		_ = json.Unmarshal(raw, &r.Body)
	}
	return r
}

func (r response) str(key string) string { s, _ := r.Body[key].(string); return s }

func expectStatus(t *testing.T, what string, r response, want int) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("%s: status %d, want %d (body %v)", what, r.Status, want, r.Body)
	}
}

var userSeq int

func signUp(t *testing.T, name string) (client, string) {
	t.Helper()
	userSeq++
	email := fmt.Sprintf("%s-%d-%d@e2e.test", name, time.Now().UnixNano(), userSeq)

	anon := client{t: t}
	reg := anon.do(http.MethodPost, "/register", map[string]string{"email": email, "password": "password123", "name": name})
	expectStatus(t, "register", reg, http.StatusCreated)

	login := anon.do(http.MethodPost, "/login", map[string]string{"email": email, "password": "password123"})
	expectStatus(t, "login", login, http.StatusOK)

	return client{t: t, token: login.str("access_token")}, reg.str("id")
}

// ---------- tests ----------

func TestIntegration_E2E_TaskWorkflow(t *testing.T) {
	owner, _ := signUp(t, "Owner")
	dev, devID := signUp(t, "Dev")
	outsider, _ := signUp(t, "Outsider")

	team := owner.do(http.MethodPost, "/teams", map[string]string{"name": "E2E Team"})
	expectStatus(t, "create team", team, http.StatusCreated)
	teamID := team.str("id")

	expectStatus(t, "invite", owner.do(http.MethodPost, "/teams/"+teamID+"/invite", map[string]string{"user_id": devID}), http.StatusCreated)

	task := owner.do(http.MethodPost, "/tasks", map[string]any{"team_id": teamID, "title": "Ship it", "assignee_id": devID, "priority": "high"})
	expectStatus(t, "create task", task, http.StatusCreated)
	taskID := task.str("id")

	// Listing twice: the second response is served from the Redis cache and
	// must be identical.
	first := dev.do(http.MethodGet, "/tasks?team_id="+teamID, nil)
	second := dev.do(http.MethodGet, "/tasks?team_id="+teamID, nil)
	expectStatus(t, "list", first, http.StatusOK)
	if fmt.Sprint(first.Body) != fmt.Sprint(second.Body) {
		t.Fatalf("cached page differs:\n%v\n%v", first.Body, second.Body)
	}

	// Isolation: an outsider can neither list nor read the team's tasks.
	expectStatus(t, "outsider list", outsider.do(http.MethodGet, "/tasks?team_id="+teamID, nil), http.StatusForbidden)
	expectStatus(t, "outsider get", outsider.do(http.MethodGet, "/tasks/"+taskID, nil), http.StatusNotFound)
	expectStatus(t, "outsider stats", outsider.do(http.MethodGet, "/teams/"+teamID+"/stats", nil), http.StatusForbidden)

	// Assignee: status yes, title no.
	r := dev.do(http.MethodPut, "/tasks/"+taskID, map[string]any{"title": "Hijacked", "version": 1})
	expectStatus(t, "assignee edits title", r, http.StatusForbidden)
	if r.str("code") != "forbidden" || r.str("request_id") == "" {
		t.Fatalf("error body = %v, want code=forbidden and a request_id", r.Body)
	}
	expectStatus(t, "assignee closes", dev.do(http.MethodPut, "/tasks/"+taskID, map[string]any{"status": "done", "version": 1}), http.StatusOK)

	// The update invalidated the cache: the list reflects it immediately.
	list := dev.do(http.MethodGet, "/tasks?team_id="+teamID+"&status=done", nil)
	if total, _ := list.Body["total"].(float64); total != 1 {
		t.Fatalf("done tasks after update = %v, want 1 (stale cache?)", list.Body["total"])
	}

	// History has both entries, newest first, with before/after values.
	history := dev.do(http.MethodGet, "/tasks/"+taskID+"/history", nil)
	expectStatus(t, "history", history, http.StatusOK)
	if len(history.List) != 2 {
		t.Fatalf("history entries = %d, want 2", len(history.List))
	}
	latest := history.List[0].(map[string]any)
	status := latest["changes"].(map[string]any)["status"].(map[string]any)
	if latest["action"] != "updated" || status["old"] != "todo" || status["new"] != "done" {
		t.Fatalf("latest history = %v", latest)
	}

	expectStatus(t, "comment", dev.do(http.MethodPost, "/tasks/"+taskID+"/comments", map[string]string{"content": "done"}), http.StatusCreated)

	stats := owner.do(http.MethodGet, "/teams/"+teamID+"/stats", nil)
	expectStatus(t, "stats", stats, http.StatusOK)
	if stats.Body["total_comments"] != 1.0 || stats.Body["tasks_by_status"].(map[string]any)["done"] != 1.0 {
		t.Fatalf("stats = %v", stats.Body)
	}
	expectStatus(t, "member stats", dev.do(http.MethodGet, "/teams/"+teamID+"/stats", nil), http.StatusForbidden)
}

// Concurrent writers holding the same version: the row lock serialises
// them and exactly one wins; the rest get 409 and nothing is lost.
func TestIntegration_E2E_ConcurrentUpdatesDoNotLoseChanges(t *testing.T) {
	owner, _ := signUp(t, "Owner")
	team := owner.do(http.MethodPost, "/teams", map[string]string{"name": "Race"})
	teamID := team.str("id")
	task := owner.do(http.MethodPost, "/tasks", map[string]any{"team_id": teamID, "title": "Contended"})
	taskID := task.str("id")

	const writers = 10
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		statuses = map[int]int{}
		winner   string
	)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			title := fmt.Sprintf("writer %d", i)
			r := owner.do(http.MethodPut, "/tasks/"+taskID, map[string]any{"title": title, "version": 1})
			mu.Lock()
			defer mu.Unlock()
			statuses[r.Status]++
			if r.Status == http.StatusOK {
				winner = title
			}
			if r.Status == http.StatusConflict && r.str("code") != "version_conflict" {
				t.Errorf("409 code = %q", r.str("code"))
			}
		}()
	}
	wg.Wait()

	if statuses[http.StatusOK] != 1 || statuses[http.StatusConflict] != writers-1 {
		t.Fatalf("statuses = %v, want 1×200 and %d×409", statuses, writers-1)
	}

	final := owner.do(http.MethodGet, "/tasks/"+taskID, nil)
	if final.str("title") != winner || final.Body["version"] != 2.0 {
		t.Fatalf("final task = %v, want winner %q at version 2", final.Body, winner)
	}

	history := owner.do(http.MethodGet, "/tasks/"+taskID+"/history", nil)
	if len(history.List) != 2 { // created + exactly one update
		t.Fatalf("history entries = %d, want 2", len(history.List))
	}
}

func TestIntegration_E2E_ProbesAndMetrics(t *testing.T) {
	for _, path := range []string{"/livez", "/readyz"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, http.NoBody)
		resp, err := application.server.App().Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
	resp, err := application.server.App().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("taskmgmt_http_requests_total")) {
		t.Fatalf("/metrics = %d", resp.StatusCode)
	}
}
