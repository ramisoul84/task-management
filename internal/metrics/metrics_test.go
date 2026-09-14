package metrics_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ramisoul84/task-management/internal/metrics"
)

func TestHTTPMetricsUseRouteTemplates(t *testing.T) {
	m := metrics.New("test")
	app := fiber.New()
	app.Use(m.HTTP())
	app.Get("/tasks/:id", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})))

	for _, id := range []string{"a", "b", "c"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tasks/"+id, http.NoBody)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	m.CacheLookup("task_list", "hit")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	out := string(body)

	for _, want := range []string{
		`taskmgmt_http_requests_total{method="GET",route="/tasks/:id",status="204"} 3`,
		`taskmgmt_cache_lookups_total{cache="task_list",result="hit"} 1`,
		`taskmgmt_build_info{version="test"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
	if strings.Contains(out, `route="/tasks/a"`) {
		t.Error("raw paths must not be used as labels")
	}
}
