package provider

import (
	"github.com/waldner/external-dns-webhook-he/pkg/client"
	"github.com/waldner/external-dns-webhook-he/pkg/config"
	"sigs.k8s.io/external-dns/endpoint"
)

func NewMockProvider(config *config.Config, domainFilter *endpoint.DomainFilter) *Provider {
	client := client.NewMockClient(config)
	provider, _ := NewProvider(client, domainFilter)
	return provider
}
