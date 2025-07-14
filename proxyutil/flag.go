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
	InsecureListenAddress string         `yaml:"insecure_listen_addr"`
	InternalListenAddress string         `yaml:"internal_listen_addr"`
	Upstream              string         `yaml:"upstream"`
	ProxyPaths            []string       `yaml:"proxy_paths"`
	PassthroughPaths      []string       `yaml:"passthrough_paths"`
	ProxyConfig           proxymw.Config `yaml:"proxymw_config"`
	ReadTimeout           time.Duration  `yaml:"proxy_read_timeout"`
	WriteTimeout          time.Duration  `yaml:"proxy_write_timeout"`
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
	cfg := Config{}
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	var (
		blockPatterns         StringSlice
		bpQueries             StringSlice
		bpQueryNames          StringSlice
		bpWarnThresholds      Float64Slice
		bpEmergencyThresholds Float64Slice
		proxyPaths            string
		passthroughPaths      string
		configFile            string
	)

	// Config file
	flags.StringVar(&configFile, "config-file", "", "Path to proxy configuration file")

	// Server settings
	flags.StringVar(
		&cfg.InsecureListenAddress,
		"insecure-listen-address",
		"",
		"HTTP proxy server listen address",
	)
	flags.StringVar(
		&cfg.InternalListenAddress,
		"internal-listen-address",
		"",
		"Internal metrics server listen address",
	)
	flags.DurationVar(&cfg.ReadTimeout, "proxy-read-timeout", 5*time.Minute, "HTTP read timeout")
	flags.DurationVar(&cfg.WriteTimeout, "proxy-write-timeout", 5*time.Minute, "HTTP write timeout")
	flags.StringVar(&cfg.Upstream, "upstream", "", "Upstream URL to proxy to")

	// Feature flags
	flags.BoolVar(
		&cfg.ProxyConfig.EnableCriticality,
		"enable-criticality",
		false,
		"Enable criticality header processing",
	)
	flags.BoolVar(&cfg.ProxyConfig.EnableJitter, "enable-jitter", false, "Enable request jitter")
	flags.DurationVar(
		&cfg.ProxyConfig.JitterDelay,
		"jitter-delay",
		0,
		"Random jitter delay duration",
	)
	flags.BoolVar(
		&cfg.ProxyConfig.EnableObserver,
		"enable-observer",
		false,
		"Enable middleware metrics collection",
	)

	// Blocker settings
	flags.BoolVar(
		&cfg.ProxyConfig.EnableBlocker,
		"enable-blocker",
		false,
		"Enable http header request blocking",
	)
	flags.Var(
		&blockPatterns,
		"block-pattern",
		"Header with regex matcher to block. Ex. `X-user-agent=service-to-block.*`",
	)

	// Backpressure settings
	bp := &cfg.ProxyConfig.BackpressureConfig
	flags.BoolVar(
		&bp.EnableBackpressure,
		"enable-bp",
		false,
		"Enable backpressure-based throttling",
	)
	flags.IntVar(&bp.CongestionWindowMin, "bp-min-window", 0, "Minimum concurrent query limit")
	flags.IntVar(&bp.CongestionWindowMax, "bp-max-window", 0, "Maximum concurrent query limit")
	flags.StringVar(
		&bp.BackpressureMonitoringURL,
		"bp-monitoring-url",
		"",
		"Backpressure metrics endpoint",
	)
	flags.Var(&bpQueries, "bp-query", "PromQL query for downstream failures")
	flags.Var(&bpQueryNames, "bp-query-name", "Human-readable name for backpressure query")
	flags.Var(&bpWarnThresholds, "bp-warn", "Warning threshold for throttling")
	flags.Var(&bpEmergencyThresholds, "bp-emergency", "Emergency threshold for maximum throttling")
	flags.BoolVar(
		&bp.EnableLowCostBypass,
		"enable-low-cost-bypass",
		false,
		"Enable low-cost realtime PromQL to bypass backpressure",
	)

	// Path settings
	flags.StringVar(&proxyPaths, "proxy-paths", "", "Comma-separated list of paths to proxy")
	flags.StringVar(
		&passthroughPaths,
		"passthrough-paths",
		"",
		"Comma-separated list of paths to pass through",
	)

	if err := flags.Parse(os.Args[1:]); err != nil {
		return Config{}, err
	}

	if configFile != "" {
		return ParseConfigFile(configFile)
	}

	cfg.ProxyConfig.BlockPatterns = blockPatterns

	var err error
	if bp.BackpressureQueries, err = proxymw.ParseBackpressureQueries(
		bpQueries, bpQueryNames, bpWarnThresholds, bpEmergencyThresholds,
	); err != nil {
		return Config{}, err
	}
	if cfg.ProxyPaths, err = parsePaths(proxyPaths); err != nil {
		return Config{}, err
	}
	if cfg.PassthroughPaths, err = parsePaths(passthroughPaths); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func ParseConfigEnvironment() (Config, error) {
	cfg := Config{}
	var err error

	cfg.Upstream = os.Getenv("UPSTREAM")

	if cfg.ProxyPaths, err = parsePaths(os.Getenv("PROXY_PATHS")); err != nil {
		return Config{}, err
	}
	if cfg.PassthroughPaths, err = parsePaths(os.Getenv("PASSTHROUGH_PATHS")); err != nil {
		return Config{}, err
	}

	if cfg.ProxyConfig.EnableJitter, err = getBoolEnv("PROXYMW_ENABLE_JITTER"); err != nil {
		return Config{}, err
	}
	if cfg.ProxyConfig.JitterDelay, err = getDurationEnv("PROXYMW_JITTER_DELAY"); err != nil {
		return Config{}, err
	}

	return cfg, nil
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

	pathList := []string{}
	for _, path := range strings.Split(paths, ",") {
		u, err := url.Parse("http://example.com" + path)
		if err != nil || u.Path != path || path == "" || path == "/" {
			return nil, fmt.Errorf("invalid path %q in path list %q", path, paths)
		}
		pathList = append(pathList, path)
	}
	return pathList, nil
}

func ParseConfigFile(configFile string) (Config, error) {
	return ParseFile[Config](configFile)
}

func ParseProxyConfigFile(configFile string) (proxymw.Config, error) {
	return ParseFile[proxymw.Config](configFile)
}

func ParseFile[T any](configFile string) (cfg T, err error) {
	file, err := os.Open(configFile) // nolint:gosec // input configuration file
	if err != nil {
		return cfg, fmt.Errorf("error opening config file: %w", err)
	}
	defer file.Close() //nolint:errcheck // ignore body close

	if err := yaml.NewDecoder(file).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("error decoding YAML: %w", err)
	}

	return cfg, nil
}
