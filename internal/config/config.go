// Package config reads the BFF's boot configuration from the environment.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// UpstreamEnv names the mandatory base-URL variable of each upstream service.
var UpstreamEnv = map[composition.Service]string{
	composition.ProductDiscovery: "PRODUCT_DISCOVERY_URL",
	composition.DeliveryPromise:  "DELIVERY_PROMISE_URL",
	composition.ShoppingCart:     "SHOPPING_CART_URL",
	composition.Checkout:         "CHECKOUT_URL",
	composition.CustomerOrdering: "CUSTOMER_ORDERING_URL",
	composition.Pricing:          "PRICING_URL",
}

// Config is the validated boot configuration.
type Config struct {
	Addr         string
	Upstreams    map[composition.Service]string
	Budgets      composition.Budgets
	CookieSecure bool
}

// Load reads and validates the environment. Every missing upstream URL is
// reported together. getenv is os.Getenv in production.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:         ":" + valueOr(getenv("PORT"), "8080"),
		Upstreams:    map[composition.Service]string{},
		Budgets:      composition.DefaultBudgets(),
		CookieSecure: true,
	}
	var problems []string
	for _, s := range composition.Services() {
		name := UpstreamEnv[s]
		if v := strings.TrimSpace(getenv(name)); v != "" {
			cfg.Upstreams[s] = v
		} else {
			problems = append(problems, name+" is required")
		}
	}
	for name, dst := range map[string]*time.Duration{
		"BUDGET_QUERY_MS": &cfg.Budgets.Query, "BUDGET_COMMAND_MS": &cfg.Budgets.Command, "BUDGET_CONFIRM_MS": &cfg.Budgets.Confirm,
	} {
		if v := getenv(name); v != "" {
			ms, err := strconv.Atoi(v)
			if err != nil || ms <= 0 {
				problems = append(problems, name+" must be a positive number of milliseconds")
				continue
			}
			*dst = time.Duration(ms) * time.Millisecond
		}
	}
	if v := getenv("COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			problems = append(problems, "COOKIE_SECURE must be a boolean")
		}
		cfg.CookieSecure = b
	}
	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
