package proxymw

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const (
	BlockerProxyType = "blocker"
)

type BlockerConfig struct {
	EnableBlocker bool `yaml:"enable_blocker"`
	// BlockPatterns is a list of header values to block and looks like `<header>=<pattern>`.
	// Ex. `X-user-agent=service-to-block.*`
	BlockPatterns []string `yaml:"block_patterns"`
}

func (q BlockerConfig) Validate() error {
	return ValidateBlockPatterns(q.BlockPatterns)
}

type Blocker struct {
	patterns map[string]*regexp.Regexp
	client   ProxyClient
}

var _ ProxyClient = &Blocker{}

func ValidateBlockPatterns(patterns []string) error {
	for _, pattern := range patterns {
		patternParts := strings.SplitN(pattern, "=", 2)
		if len(patternParts) != 2 {
			return fmt.Errorf("pattern %q did not match `<header>=<regex>`", pattern)
		}

		_, err := regexp.Compile(patternParts[1])
		if err != nil {
			return err
		}

		if patternParts[0] == "" {
			return fmt.Errorf("header is empty for pattern %q", pattern)
		}
	}
	return nil
}

func NewBlocker(client ProxyClient, cfg BlockerConfig) *Blocker {
	blockPatterns := map[string]*regexp.Regexp{}
	for _, pattern := range cfg.BlockPatterns {
		patternParts := strings.SplitN(pattern, "=", 2)
		blockPatterns[patternParts[0]] = regexp.MustCompile(patternParts[1])
	}
	return &Blocker{
		patterns: blockPatterns,
		client:   client,
	}
}

func (b *Blocker) Init(ctx context.Context) {
	b.client.Init(ctx)
}

func (b *Blocker) Next(rr Request) error {
	headers := rr.Request().Header
	for header, regex := range b.patterns {
		for _, val := range headers[header] {
			if regex.MatchString(val) {
				msg := "header %s, value %s blocked by regex %s"
				return BlockErr(BlockerProxyType, msg, header, val, regex.String())
			}
		}
	}
	return b.client.Next(rr)
}
