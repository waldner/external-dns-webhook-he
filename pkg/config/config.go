package config

import (
	"fmt"
	"strings"

	"github.com/caarlos0/env/v8"
	log "github.com/sirupsen/logrus"
	"github.com/waldner/external-dns-webhook-he/pkg/common"
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
	Username string
	Password string
	Url      string
}

func NewConfig() (*Config, *endpoint.DomainFilter, error) {

	conf := envConfig{}
	if err := env.Parse(&conf); err != nil {
		log.Fatalf("NewConfig: error reading configuration from environment: %s", err)
	}

	if conf.Username == "" {
		log.Fatal("NewConfig: empty username supplied")
	}
	if conf.Password == "" {
		log.Fatal("NewConfig: empty password supplied")
	}

	// regex matches take precedence over plain text matching
	if conf.RegexDomainFilter != "" {
		msg := fmt.Sprintf("Using regexp domain filter: '%s', ", conf.RegexDomainFilter)
		if conf.RegexDomainExclude != "" {
			msg += fmt.Sprintf("with exclusion: '%s', ", conf.RegexDomainExclude)
		}
		log.Info(msg)
	} else {
		msg := fmt.Sprintf("Using plain domain filter with domains '%s'", strings.Join(conf.DomainFilter, ","))
		if conf.DomainFilterExclude != nil && len(conf.DomainFilterExclude) > 0 {
			msg += fmt.Sprintf("with exclusions: '%s', ", strings.Join(conf.DomainFilterExclude, ","))
		}
	}

	domainFilter := common.CreateDomainFilter(conf.RegexDomainFilter, conf.RegexDomainExclude, conf.DomainFilter, conf.DomainFilterExclude)

	return &Config{
		Username: conf.Username,
		Password: conf.Password,
		Url:      conf.Url,
	}, domainFilter, nil

}
