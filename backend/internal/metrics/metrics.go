package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// Zero-dependency Prometheus exposition: счётчики запросов по методу/пути/коду
// и гистограмма длительностей (бакеты в секундах).

var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type key struct {
	method string
	route  string
	code   int
}

type histogram struct {
	count uint64
	sum   float64
	bins  []uint64
}

type Collector struct {
	mu   sync.Mutex
	reqs map[key]uint64
	hist map[key]*histogram
}

var Default = &Collector{reqs: map[key]uint64{}, hist: map[key]*histogram{}}

func normalizeRoute(c echo.Context) string {
	if r := c.Path(); r != "" {
		return r
	}
	return "unknown"
}

// Middleware считает запросы. Вешается до остальных middleware.
func Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		err := next(c)
		code := c.Response().Status
		if he, ok := err.(*echo.HTTPError); ok && he.Code != 0 {
			code = he.Code
		}
		Default.observe(c.Request().Method, normalizeRoute(c), code, time.Since(start).Seconds())
		return err
	}
}

func (col *Collector) observe(method, route string, code int, dur float64) {
	k := key{method, route, code}
	col.mu.Lock()
	defer col.mu.Unlock()
	col.reqs[k]++
	h := col.hist[k]
	if h == nil {
		h = &histogram{bins: make([]uint64, len(buckets))}
		col.hist[k] = h
	}
	h.count++
	h.sum += dur
	for i, b := range buckets {
		if dur <= b {
			h.bins[i]++
			break
		}
	}
}

// Render отдаёт exposition format 0.0.4.
func (col *Collector) Render() string {
	var sb strings.Builder
	sb.WriteString("# HELP rms_http_requests_total Total HTTP requests.\n")
	sb.WriteString("# TYPE rms_http_requests_total counter\n")
	keys := make([]key, 0, len(col.reqs))
	col.mu.Lock()
	for k := range col.reqs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].code < keys[j].code
	})
	for _, k := range keys {
		fmt.Fprintf(&sb, "rms_http_requests_total{method=%q,route=%q,code=%q} %d\n",
			k.method, k.route, fmt.Sprint(k.code), col.reqs[k])
		h := col.hist[k]
		if h == nil {
			continue
		}
		fmt.Fprintf(&sb, "rms_http_request_duration_seconds_count{method=%q,route=%q} %d\n",
			k.method, k.route, h.count)
		fmt.Fprintf(&sb, "rms_http_request_duration_seconds_sum{method=%q,route=%q} %f\n",
			k.method, k.route, h.sum)
		var cum uint64
		for i, b := range buckets {
			cum += h.bins[i]
			fmt.Fprintf(&sb, "rms_http_request_duration_seconds_bucket{method=%q,route=%q,le=%q} %d\n",
				k.method, k.route, fmt.Sprint(b), cum)
		}
		fmt.Fprintf(&sb, "rms_http_request_duration_seconds_bucket{method=%q,route=%q,le=%q} %d\n",
			k.method, k.route, "+Inf", h.count)
	}
	col.mu.Unlock()
	return sb.String()
}
