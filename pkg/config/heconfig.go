package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/caarlos0/env/v8"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
)

type envConfig struct {
	Username            string   `env:"WEBHOOK_HE_USERNAME" envDefault:""`
	Password            string   `env:"WEBHOOK_HE_PASSWORD" envDefault:""`
	Url                 string   `env:"WEBHOOK_HE_URL" envDefault:"https://dns.he.net"`
	DomainFilter        []string `env:"WEBHOOK_HE_DOMAIN_FILTER" envDefault:""`
	DomainFilterExclude []string `env:"WEBHOOK_HE_DOMAIN_FILTER_EXCLUDE" envDefault:""`
	RegexDomainFilter   string   `env:"WEBHOOK_HE_REGEXP_DOMAIN_FILTER" envDefault:""`
	RegexDomainExclude  string   `env:"WEBHOOK_HE_REGEXP_DOMAIN_FILTER_EXCLUDE" envDefault:""`
}

type Config struct {
	Username     string
	Password     string
	Url          string
	DomainFilter endpoint.DomainFilter
}

func NewHEConfig() (*Config, error) {

	conf := envConfig{}
	if err := env.Parse(&conf); err != nil {
		log.Fatalf("NewHEConfig: error reading configuration from environment: %s", err)
	}

	if conf.Username == "" {
		log.Fatal("NewHEConfig: empty username supplied")
	}
	if conf.Password == "" {
		log.Fatal("NewHeConfig: empty password supplied")
	}

	var domainFilter endpoint.DomainFilter

	// regex matches take precedence over plain text matching
	if conf.RegexDomainFilter != "" {
		msg := fmt.Sprintf("Using regexp domain filter: '%s', ", conf.RegexDomainFilter)
		if conf.RegexDomainExclude != "" {
			msg += fmt.Sprintf("with exclusion: '%s', ", conf.RegexDomainExclude)
		}
		log.Info(msg)
		domainFilter = endpoint.NewRegexDomainFilter(
			regexp.MustCompile(conf.RegexDomainFilter),
			regexp.MustCompile(conf.RegexDomainExclude),
		)
	} else {
		msg := fmt.Sprintf("Using plain domain filter with domains '%s'", strings.Join(conf.DomainFilter, ","))
		if conf.DomainFilterExclude != nil && len(conf.DomainFilterExclude) > 0 {
			msg += fmt.Sprintf("with exclusions: '%s', ", strings.Join(conf.DomainFilterExclude, ","))
		}
		log.Info(msg)
		domainFilter = endpoint.NewDomainFilterWithExclusions(conf.DomainFilter, conf.DomainFilterExclude)
	}

	return &Config{
		Username:     conf.Username,
		Password:     conf.Password,
		Url:          conf.Url,
		DomainFilter: domainFilter,
	}, nil

}
