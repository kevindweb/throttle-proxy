package proxymw

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/thanos-io/promql-engine/logicalplan"
	"github.com/thanos-io/promql-engine/query"
)

const ObjectStorageThreshold = 100
const DefaultRangeStep = time.Second * 30

type intermediateQuery struct {
	query string
	start time.Time
	end   time.Time
	step  time.Duration
}

func LowCostRequest(rr Request) (bool, error) {
	cost, err := QueryCost(rr)
	return cost < ObjectStorageThreshold, err
}

func QueryCost(rr Request) (int, error) {
	q, err := queryFromRequest(rr)
	if err != nil {
		return 0, err
	}

	expr, err := parser.NewParser(q.query).ParseExpr()
	if err != nil {
		return 0, err
	}

	planOpts := logicalplan.PlanOptions{}
	qOpts := &query.Options{
		Start: q.start,
		End:   q.end,
		Step:  q.step,
		// Thanos defaults
		LookbackDelta: 5 * time.Minute,
	}
	min, _ := logicalplan.NewFromAST(expr, qOpts, planOpts).MinMaxTime(qOpts)
	twoHoursAgo := time.Now().UTC().Add(-time.Hour * 2).UnixMilli()
	if min < twoHoursAgo {
		return ObjectStorageThreshold, nil
	}
	return 0, nil
}

func queryFromRequest(rr Request) (intermediateQuery, error) {
	req := rr.Request()
	if req == nil {
		return intermediateQuery{}, errors.New("nil HTTP request when parsing promql")
	}

	req, err := DupRequest(req)
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("error duplicating request for parsing: %w", err)
	}

	if req.URL == nil {
		return intermediateQuery{}, errors.New("nil URL when parsing promql")
	}

	switch req.URL.Path {
	case "/api/v1/query":
		return queryFromInstant(req)
	case "/api/v1/query_range":
		return queryFromRange(req)
	default:
		return intermediateQuery{}, fmt.Errorf(
			"can only handle instant or range query, found %s", req.URL.Path,
		)
	}
}

func queryFromInstant(req *http.Request) (intermediateQuery, error) {
	err := req.ParseForm()
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("bad request in instant query %v", err)
	}

	query := req.Form.Get("query")
	ts := req.Form.Get("time")
	return parseRequestArguments(query, ts, ts, "0")
}

func queryFromRange(req *http.Request) (intermediateQuery, error) {
	err := req.ParseForm()
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("bad request in range query %v", err)
	}

	query := req.Form.Get("query")
	start := req.Form.Get("start")
	end := req.Form.Get("end")
	step := req.Form.Get("step")
	return parseRequestArguments(query, start, end, step)
}

func parseRequestArguments(query string, start string, end string, step string) (intermediateQuery, error) {
	defaultTime := time.Now().UTC()
	startTime, err := parseDefaultTime(start, defaultTime)
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("error parsing start time %v", err)
	}

	endTime, err := parseDefaultTime(end, defaultTime)
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("error parsing end time %v", err)
	}

	stepDuration, err := parseDefaultDuration(step, DefaultRangeStep)
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("error parsing step %v", err)
	}

	return intermediateQuery{
		query: query,
		start: startTime,
		end:   endTime,
		step:  stepDuration,
	}, nil
}

func parseDefaultTime(s string, d time.Time) (time.Time, error) {
	if s != "" {
		return parseTime(s)
	}
	return d, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		ns = math.Round(ns*1000) / 1000
		return time.Unix(int64(s), int64(ns*float64(time.Second))), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

func parseDefaultDuration(s string, d time.Duration) (time.Duration, error) {
	if s != "" {
		return parseDuration(s)
	}
	return d, nil
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		ts := d * float64(time.Second)
		if ts > float64(math.MaxInt64) || ts < float64(math.MinInt64) {
			return 0, fmt.Errorf("cannot parse %q to a valid duration. It overflows int64", s)
		}
		return time.Duration(ts), nil
	}
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid duration", s)
}
