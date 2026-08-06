package pagination

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// parseWithQuery runs Parse against a request carrying the given query string.
func parseWithQuery(t *testing.T, query string) Request {
	t.Helper()

	app := fiber.New()
	var got Request
	app.Get("/", func(c *fiber.Ctx) error {
		got = Parse(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/?"+query, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	_ = resp.Body.Close()
	return got
}

func TestParseDefaults(t *testing.T) {
	got := parseWithQuery(t, "")
	if got.Page != DefaultPage || got.PerPage != DefaultPerPage {
		t.Errorf("defaults = page %d, per_page %d", got.Page, got.PerPage)
	}
	if got.SortDir != "desc" {
		t.Errorf("default sort direction = %q, want desc", got.SortDir)
	}
}

func TestParseClampsPerPage(t *testing.T) {
	if got := parseWithQuery(t, "per_page=5000"); got.PerPage != MaxPerPage {
		t.Errorf("per_page = %d, want clamped to %d", got.PerPage, MaxPerPage)
	}
	if got := parseWithQuery(t, "per_page=0"); got.PerPage != DefaultPerPage {
		t.Errorf("per_page=0 gave %d, want the default", got.PerPage)
	}
	if got := parseWithQuery(t, "per_page=-3"); got.PerPage != DefaultPerPage {
		t.Errorf("negative per_page gave %d, want the default", got.PerPage)
	}
	if got := parseWithQuery(t, "page=abc"); got.Page != DefaultPage {
		t.Errorf("non-numeric page gave %d, want the default", got.Page)
	}
}

func TestParseSortSyntaxes(t *testing.T) {
	cases := []struct {
		query   string
		wantBy  string
		wantDir string
	}{
		{"sort=-created_at", "created_at", "desc"},
		{"sort=%2Bprice", "price", "asc"},
		{"sort=price&order=asc", "price", "asc"},
		{"sort=price&order=desc", "price", "desc"},
		{"sort=price&order=sideways", "price", "desc"},
	}
	for _, tc := range cases {
		got := parseWithQuery(t, tc.query)
		if got.SortBy != tc.wantBy || got.SortDir != tc.wantDir {
			t.Errorf("%s → by %q dir %q, want %q %q",
				tc.query, got.SortBy, got.SortDir, tc.wantBy, tc.wantDir)
		}
	}
}

func TestOrderClauseRejectsUnknownColumns(t *testing.T) {
	allowed := map[string]string{
		"created_at": "loads.created_at",
		"price":      "loads.price_amount",
	}
	fallback := "loads.published_at DESC"

	// An allowlisted column is mapped to its qualified SQL name.
	req := Request{SortBy: "price", SortDir: "asc"}
	if got := req.OrderClause(allowed, fallback); got != "loads.price_amount ASC" {
		t.Errorf("OrderClause = %q", got)
	}

	// Anything not allowlisted — including an injection attempt — falls back.
	for _, sortBy := range []string{"", "unknown", "id; DROP TABLE loads", "1=1"} {
		req := Request{SortBy: sortBy, SortDir: "desc"}
		if got := req.OrderClause(allowed, fallback); got != fallback {
			t.Errorf("OrderClause(%q) = %q, want the fallback", sortBy, got)
		}
	}
}

func TestOffsetAndLimit(t *testing.T) {
	req := Request{Page: 3, PerPage: 20}
	if req.Offset() != 40 {
		t.Errorf("Offset() = %d, want 40", req.Offset())
	}
	if req.Limit() != 20 {
		t.Errorf("Limit() = %d, want 20", req.Limit())
	}
	if first := (Request{Page: 1, PerPage: 20}).Offset(); first != 0 {
		t.Errorf("first page offset = %d, want 0", first)
	}
}

func TestTotalPages(t *testing.T) {
	cases := []struct {
		total   int64
		perPage int
		want    int
	}{
		{0, 20, 0},
		{1, 20, 1},
		{20, 20, 1},
		{21, 20, 2},
		{134, 20, 7},
		{10, 0, 0},
	}
	for _, tc := range cases {
		if got := TotalPages(tc.total, tc.perPage); got != tc.want {
			t.Errorf("TotalPages(%d, %d) = %d, want %d", tc.total, tc.perPage, got, tc.want)
		}
	}
}
