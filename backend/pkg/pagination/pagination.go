// Package pagination parses list-endpoint query parameters into a safe,
// bounded request that repositories can apply directly.
package pagination

import (
	"math"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// Request is a validated page/sort request.
type Request struct {
	Page    int
	PerPage int
	SortBy  string
	SortDir string // asc | desc
	Search  string
}

// Offset is the SQL OFFSET for this page.
func (r Request) Offset() int { return (r.Page - 1) * r.PerPage }

// Limit is the SQL LIMIT for this page.
func (r Request) Limit() int { return r.PerPage }

// OrderClause returns a safe "column direction" string, or the fallback when the
// requested column is not in the allowlist. Column names never come from user
// input unchecked — this is the only place sorting is turned into SQL.
func (r Request) OrderClause(allowed map[string]string, fallback string) string {
	col, ok := allowed[r.SortBy]
	if !ok {
		return fallback
	}
	dir := "ASC"
	if r.SortDir == "desc" {
		dir = "DESC"
	}
	return col + " " + dir
}

// Parse reads page, per_page, sort, order and q from the query string.
// Accepted sort syntax: ?sort=created_at&order=desc or ?sort=-created_at
func Parse(c *fiber.Ctx) Request {
	req := Request{
		Page:    DefaultPage,
		PerPage: DefaultPerPage,
		SortDir: "desc",
	}

	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			req.Page = n
		}
	}
	if v := c.Query("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			req.PerPage = min(n, MaxPerPage)
		}
	}

	sort := strings.TrimSpace(c.Query("sort"))
	if after, found := strings.CutPrefix(sort, "-"); found {
		req.SortBy, req.SortDir = after, "desc"
	} else if after, found := strings.CutPrefix(sort, "+"); found {
		req.SortBy, req.SortDir = after, "asc"
	} else {
		req.SortBy = sort
	}

	if order := strings.ToLower(strings.TrimSpace(c.Query("order"))); order == "asc" || order == "desc" {
		req.SortDir = order
	}

	req.Search = strings.TrimSpace(c.Query("q"))
	return req
}

// TotalPages computes the page count for a total row count.
func TotalPages(total int64, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(perPage)))
}
