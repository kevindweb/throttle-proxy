// Package proxyutil handles parsing logic for proxymw configs
package proxyutil

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

type Config struct {
	// InsecureListenAddress is the address the proxy HTTP server should listen on
	InsecureListenAddress string `yaml:"insecure_listen_addr"`
	// InternalListenAddress is the address the HTTP server should listen on for pprof and metrics
	InternalListenAddress string `yaml:"internal_listen_addr"`
	// Upstream is the upstream URL to proxy to
	Upstream string `yaml:"upstream"`
	// ProxyPaths is the list of paths to throttle with proxy settings
	ProxyPaths []string `yaml:"proxy_paths"`
	// PassthroughPaths is a list of paths to pass through instead of applying proxy settings
	PassthroughPaths []string       `yaml:"passthrough_paths"`
	ProxyConfig      proxymw.Config `yaml:"proxymw_config"`
	ReadTimeout      string         `yaml:"proxy_read_timeout"`
	WriteTimeout     string         `yaml:"proxy_write_timeout"`
}

type StringSlice []string

func (s *StringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *StringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type Float64Slice []float64

func (f *Float64Slice) String() string {
	values := make([]string, len(*f))
	for i, v := range *f {
		values[i] = fmt.Sprintf("%g", v)
	}
	return strings.Join(values, ",")
}

func (f *Float64Slice) Set(value string) error {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	*f = append(*f, v)
	return nil
}

func ParseConfigFlags() (Config, error) {
	var (
		insecureListenAddress     string
		internalListenAddress     string
		readTimeout               string
		writeTimeout              string
		upstream                  string
		proxyPaths                string
		passthroughPaths          string
		enableBackpressure        bool
		backpressureMonitoringURL string
		bpQueries                 StringSlice
		bpQueryNames              StringSlice
		bpWarnThresholds          Float64Slice
		bpEmergencyThresholds     Float64Slice
		congestionWindowMin       int
		congestionWindowMax       int
		enableCriticality         bool
		enableJitter              bool
		jitterDelay               time.Duration
		enableObserver            bool
		configFile                string
	)

	flagset := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagset.StringVar(&configFile, "config-file", "", "Config file to initialize the proxy")
	flagset.StringVar(
		&insecureListenAddress, "insecure-listen-address", "",
		"The address the proxy HTTP server should listen on.",
	)
	flagset.StringVar(
		&internalListenAddress, "internal-listen-address", "",
		"The address the internal HTTP server should listen on to expose metrics about itself.",
	)
	flagset.StringVar(
		&readTimeout, "proxy-read-timeout", (time.Minute * 5).String(),
		"HTTP read timeout duration",
	)
	flagset.StringVar(
		&writeTimeout, "proxy-write-timeout", (time.Minute * 5).String(),
		"HTTP write timeout duration",
	)
	flagset.StringVar(&upstream, "upstream", "", "The upstream URL to proxy to.")
	flagset.BoolVar(&enableCriticality, "enable-criticality", false, "Read criticality headers")
	flagset.BoolVar(&enableJitter, "enable-jitter", false, "Use the jitter middleware")
	flagset.DurationVar(
		&jitterDelay, "jitter-delay", 0,
		"Random jitter to apply when enabled",
	)
	flagset.BoolVar(
		&enableBackpressure, "enable-bp", false,
		"Use the additive increase multiplicative decrease middleware using backpressure metrics",
	)
	flagset.IntVar(
		&congestionWindowMin, "bp-min-window", 0,
		"Min concurrent queries to passthrough regardless of spikes in backpressure.",
	)
	flagset.IntVar(
		&congestionWindowMax, "bp-max-window", 0,
		"Max concurrent queries to passthrough regardless of backpressure health.",
	)
	flagset.StringVar(
		&backpressureMonitoringURL, "bp-monitoring-url", "",
		"The address on which to read backpressure metrics with PromQL queries.",
	)
	flagset.Var(
		&bpQueries, "bp-query",
		"PromQL that signifies an increase in downstream failure",
	)
	flagset.Var(
		&bpQueryNames, "bp-query-name",
		"Name is an optional human readable field used to emit tagged metrics. "+
			"When unset, operational metrics are omitted. "+
			`When set, read warn_threshold as proxymw_bp_warn_threshold{query_name="<name>"}`,
	)
	flagset.Var(
		&bpWarnThresholds, "bp-warn",
		"Threshold that defines when the system should start backing off",
	)
	flagset.Var(
		&bpEmergencyThresholds, "bp-emergency",
		"Threshold that defines when the system should apply maximum throttling",
	)
	flagset.BoolVar(
		&enableObserver, "enable-observer", false,
		"Collect middleware latency and error metrics",
	)
	flagset.StringVar(&proxyPaths, "proxy-paths", "",
		"Comma delimited allow list of exact HTTP paths that should be allowed to hit "+
			"the upstream URL without any enforcement.")
	flagset.StringVar(&passthroughPaths, "passthrough-paths", "",
		"Comma delimited allow list of exact HTTP paths that should be allowed to hit "+
			"the upstream URL without any enforcement.")
	if err := flagset.Parse(os.Args[1:]); err != nil {
		return Config{}, err
	}

	queries, err := parseBackpressureQueries(
		bpQueries, bpQueryNames, bpWarnThresholds, bpEmergencyThresholds,
	)
	if err != nil {
		return Config{}, err
	}

	if configFile != "" {
		return ParseConfigFile(configFile)
	}

	proxyPathsList, err := parsePaths(proxyPaths)
	if err != nil {
		return Config{}, err
	}

	passthroughPathsList, err := parsePaths(passthroughPaths)
	if err != nil {
		return Config{}, err
	}

	return Config{
		InsecureListenAddress: insecureListenAddress,
		InternalListenAddress: internalListenAddress,
		ReadTimeout:           readTimeout,
		WriteTimeout:          writeTimeout,
		Upstream:              upstream,
		ProxyPaths:            proxyPathsList,
		PassthroughPaths:      passthroughPathsList,
		ProxyConfig: proxymw.Config{
			EnableCriticality: enableCriticality,
			EnableJitter:      enableJitter,
			JitterDelay:       jitterDelay,
			EnableObserver:    enableObserver,
			BackpressureConfig: proxymw.BackpressureConfig{
				EnableBackpressure:        enableBackpressure,
				BackpressureMonitoringURL: backpressureMonitoringURL,
				BackpressureQueries:       queries,
				CongestionWindowMin:       congestionWindowMin,
				CongestionWindowMax:       congestionWindowMax,
			},
		},
	}, nil
}

func ParseConfigEnvironment() (Config, error) {
	u := os.Getenv("UPSTREAM")
	proxyPaths := os.Getenv("PROXY_PATHS")
	passthroughPaths := os.Getenv("PASSTHROUGH_PATHS")

	proxyPathsList, err := parsePaths(proxyPaths)
	if err != nil {
		return Config{}, err
	}

	passthroughPathsList, err := parsePaths(passthroughPaths)
	if err != nil {
		return Config{}, err
	}

	enableJitter, err := getBoolEnv("PROXYMW_ENABLE_JITTER")
	if err != nil {
		return Config{}, err
	}

	jitterDelay, err := getDurationEnv("PROXYMW_JITTER_DELAY")
	if err != nil {
		return Config{}, err
	}

	return Config{
		Upstream:         u,
		ProxyPaths:       proxyPathsList,
		PassthroughPaths: passthroughPathsList,
		ProxyConfig: proxymw.Config{
			EnableJitter: enableJitter,
			JitterDelay:  jitterDelay,
		},
	}, nil
}

func getBoolEnv(key string) (bool, error) {
	b := os.Getenv(key)
	if b == "" {
		return false, nil
	}
	return strconv.ParseBool(b)
}

func getDurationEnv(key string) (time.Duration, error) {
	d := os.Getenv(key)
	if d == "" {
		return 0, nil
	}
	return time.ParseDuration(d)
}

func parsePaths(paths string) ([]string, error) {
	if paths == "" {
		return []string{}, nil
	}

	pathList := strings.Split(paths, ",")
	for _, path := range pathList {
		u, err := url.Parse(fmt.Sprintf("http://example.com%v", path))
		if err != nil {
			return nil, fmt.Errorf(
				"path %q is not a valid URI path, got %v", path, paths,
			)
		}
		if u.Path != path {
			return nil, fmt.Errorf(
				"path %q is not a valid URI path, got %v", path, paths,
			)
		}
		if u.Path == "" || u.Path == "/" {
			return nil, fmt.Errorf(
				"path %q is not allowed, got %v", u.Path, paths,
			)
		}
	}
	return pathList, nil
}

func parseBackpressureQueries(
	bpQueries, bpQueryNames []string, bpWarnThresholds, bpEmergencyThresholds []float64,
) ([]proxymw.BackpressureQuery, error) {
	n := len(bpQueries)
	queries := make([]proxymw.BackpressureQuery, n)
	if len(bpQueryNames) != n && len(bpQueryNames) != 0 {
		return nil, fmt.Errorf("number of backpressure query names should be 0 or %d", n)
	}

	if len(bpWarnThresholds) != n {
		return nil, fmt.Errorf("expected %d warn thresholds for %d backpressure queries", n, n)
	}

	if len(bpEmergencyThresholds) != n {
		return nil, fmt.Errorf(
			"expected %d emergency thresholds for %d backpressure queries", n, n,
		)
	}

	for i, query := range bpQueries {
		queryName := ""
		if len(bpQueryNames) > 0 {
			queryName = bpQueryNames[i]
		}
		queries[i] = proxymw.BackpressureQuery{
			Name:               queryName,
			Query:              query,
			WarningThreshold:   bpWarnThresholds[i],
			EmergencyThreshold: bpEmergencyThresholds[i],
		}
	}
	return queries, nil
}

func ParseConfigFile(configFile string) (Config, error) {
	return ParseFile[Config](configFile)
}

func ParseProxyConfigFile(configFile string) (proxymw.Config, error) {
	return ParseFile[proxymw.Config](configFile)
}

func ParseFile[T any](configFile string) (cfg T, err error) {
	// nolint:gosec // accept configuration file as input
	file, err := os.Open(configFile)
	if err != nil {
		return cfg, fmt.Errorf("error opening config file: %v", err)
	}
	defer file.Close()

	if err := yaml.NewDecoder(file).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("error decoding YAML: %v", err)
	}

	return cfg, nil
}
