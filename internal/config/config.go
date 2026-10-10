// Package config reads OpenTrade's settings from the environment. There is
// no config file format: a .env you keep locally (never committed) plus
// defaults that run the sample data out of the box.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/beiryu/opentrade/internal/rules"
)

// Config is everything the binary needs.
type Config struct {
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	DBPath  string
	DataDir string
	Listen  string

	InitialCash float64
	// Universe lists the symbols the model may trade, in order.
	Universe []string
	// Exchanges maps each symbol to its board; HOSE when unspecified.
	Exchanges map[string]rules.Exchange

	// Risk limits enforced in code after the model decides.
	MaxPositions      int
	MaxPositionPct    float64 // of equity, per symbol
	MaxOrdersPerCycle int
}

// Defaults runs the synthetic sample with the Vercel AI Gateway.
func Defaults() Config {
	return Config{
		LLMBaseURL:        "https://ai-gateway.vercel.sh/v1",
		LLMModel:          "deepseek/deepseek-v4.1-flash",
		DBPath:            "data/local/opentrade.db",
		DataDir:           "data/sample",
		Listen:            "127.0.0.1:8080",
		InitialCash:       100_000_000,
		Universe:          []string{"FPT", "VNM", "VCB", "HPG", "MWG", "VIC", "VHM", "GAS", "MSN", "TCB"},
		Exchanges:         map[string]rules.Exchange{},
		MaxPositions:      5,
		MaxPositionPct:    0.30,
		MaxOrdersPerCycle: 5,
	}
}

// Load applies OPENTRADE_* environment variables over the defaults. It
// first loads `.env` from the working directory into the process
// environment when present, without overriding variables already set.
func Load() (Config, error) {
	_ = loadDotEnv(".env")
	c := Defaults()
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
			*dst = strings.TrimSpace(v)
		}
	}
	str("OPENTRADE_LLM_BASE_URL", &c.LLMBaseURL)
	str("OPENTRADE_LLM_API_KEY", &c.LLMAPIKey)
	str("OPENTRADE_LLM_MODEL", &c.LLMModel)
	str("OPENTRADE_DB", &c.DBPath)
	str("OPENTRADE_DATA_DIR", &c.DataDir)
	str("OPENTRADE_LISTEN", &c.Listen)

	if v := os.Getenv("OPENTRADE_INITIAL_CASH"); v != "" {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, "_", ""), 64)
		if err != nil || f <= 0 {
			return c, fmt.Errorf("config: OPENTRADE_INITIAL_CASH must be a positive number, got %q", v)
		}
		c.InitialCash = f
	}
	if v := os.Getenv("OPENTRADE_UNIVERSE"); v != "" {
		c.Universe = c.Universe[:0]
		for _, s := range strings.Split(v, ",") {
			s = strings.ToUpper(strings.TrimSpace(s))
			if s != "" {
				c.Universe = append(c.Universe, s)
			}
		}
		if len(c.Universe) == 0 {
			return c, fmt.Errorf("config: OPENTRADE_UNIVERSE is empty")
		}
	}
	// OPENTRADE_EXCHANGES="SHS:HNX,BSR:UPCOM"
	if v := os.Getenv("OPENTRADE_EXCHANGES"); v != "" {
		for _, pair := range strings.Split(v, ",") {
			kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(kv) != 2 {
				return c, fmt.Errorf("config: OPENTRADE_EXCHANGES entry %q must be SYMBOL:EXCHANGE", pair)
			}
			ex, err := rules.ParseExchange(kv[1])
			if err != nil {
				return c, err
			}
			c.Exchanges[strings.ToUpper(strings.TrimSpace(kv[0]))] = ex
		}
	}
	if v := os.Getenv("OPENTRADE_MAX_POSITIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("config: OPENTRADE_MAX_POSITIONS must be a positive integer")
		}
		c.MaxPositions = n
	}
	if v := os.Getenv("OPENTRADE_MAX_POSITION_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 1 {
			return c, fmt.Errorf("config: OPENTRADE_MAX_POSITION_PCT must be in (0, 1]")
		}
		c.MaxPositionPct = f
	}
	return c, nil
}

// ExchangeOf returns the board for a symbol, HOSE by default.
func (c Config) ExchangeOf(symbol string) rules.Exchange {
	if ex, ok := c.Exchanges[strings.ToUpper(symbol)]; ok {
		return ex
	}
	return rules.HOSE
}

// InUniverse reports whether the model may trade the symbol.
func (c Config) InUniverse(symbol string) bool {
	symbol = strings.ToUpper(symbol)
	for _, s := range c.Universe {
		if s == symbol {
			return true
		}
	}
	return false
}

// loadDotEnv sets KEY=VALUE lines from path into the environment unless the
// key is already set. Comments and blank lines are skipped; surrounding
// quotes on values are removed.
func loadDotEnv(path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}
