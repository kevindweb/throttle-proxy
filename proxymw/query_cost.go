package proxymw

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/thanos-io/promql-engine/logicalplan"
	"github.com/thanos-io/promql-engine/query"
)

const ObjectStorageThreshold = 100

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
	startTime, err := parseTime(start)
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("error parsing start time %v", err)
	}

	endTime, err := parseTime(end)
	if err != nil {
		return intermediateQuery{}, fmt.Errorf("error parsing end time %v", err)
	}

	stepDuration, err := parseDuration(step)
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

func parseTime(t string) (time.Time, error) {
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return time.Time{}, err
	}

	sec := int64(math.Floor(f))
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec), nil
}

func parseDuration(s string) (time.Duration, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f < 0 {
		return 0, errors.New("duration cannot be negative")
	}
	return time.Duration(f * float64(time.Second)), nil
}
