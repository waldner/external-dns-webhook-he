package provider

import (
	"fmt"
	"regexp"

	log "github.com/sirupsen/logrus"
	"github.com/waldner/external-dns-webhook-he/pkg/common"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type Provider struct {
	client       ClientService
	domainFilter *endpoint.DomainFilter
}

type ClientService interface {
	DoLogin() error
	DoLogout() error
	GetMatchingZones(*endpoint.DomainFilter) (map[string]*common.ZoneData, error)
	GetZoneEndpoints(zone string, zoneData *common.ZoneData) ([]*endpoint.Endpoint, error)
	CreateRecords(string, *common.ZoneData, []*endpoint.Endpoint) error
	DeleteRecords(string, *common.ZoneData, []*endpoint.Endpoint) error
}

// func NewProvider(client *client.HEClient) (*Provider, error) {
func NewProvider(client ClientService, domainFilter *endpoint.DomainFilter) (*Provider, error) {
	return &Provider{
		client,
		domainFilter,
	}, nil
}

func (p *Provider) DomainFilter() *endpoint.DomainFilter {
	return p.domainFilter
}

func (p *Provider) GetAllRecords() ([]*endpoint.Endpoint, error) {

	err := p.client.DoLogin()
	if err != nil {
		return nil, fmt.Errorf("GetAllRecords: %s", err)
	}

	defer p.client.DoLogout()

	zones, err := p.client.GetMatchingZones(p.domainFilter)
	if err != nil {
		return nil, fmt.Errorf("GetAllRecords: %s", err)
	}

	log.Debugf("Matching zones according to domain filter: %v", zones)

	allEndpoints := []*endpoint.Endpoint{}

	for zone, zoneData := range zones {
		endpoints, err := p.client.GetZoneEndpoints(zone, zoneData)
		if err != nil {
			log.Errorf("GetAllRecords: error getting zone records for '%s': %s", zone, err)
			log.Warnf("Skipping zone '%s'", zone)
			continue
		}
		allEndpoints = append(allEndpoints, endpoints...)
	}

	return allEndpoints, nil

}

func (p *Provider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return endpoints, nil
}

func (p *Provider) ApplyChanges(changes *plan.Changes) error {

	log.Debugf("Changes requested (before expansion): create: %d, updateOld: %d, updateNew: %d, delete: %d", len(changes.Create), len(changes.UpdateOld), len(changes.UpdateNew), len(changes.Delete))

	if len(changes.Create) == 0 && len(changes.UpdateOld) == 0 && len(changes.UpdateNew) == 0 && len(changes.Delete) == 0 {
		log.Debugf("ApplyChanges: Nothing to do, returning")
		return nil
	}

	// updates become delete + create
	// we should also group changes by zone, so
	// all changes related to a zone are applied together later

	err := p.client.DoLogin()
	if err != nil {
		return fmt.Errorf("ApplyChanges: %s", err)
	}

	defer p.client.DoLogout()

	// group requested changes into creations and deletions
	toDelete := append(common.ExpandRecords(changes.UpdateOld), common.ExpandRecords(changes.Delete)...)
	toCreate := append(common.ExpandRecords(changes.UpdateNew), common.ExpandRecords(changes.Create)...)

	log.Debugf("Total records to be deleted: %d (%+v)", len(toDelete), toDelete)
	log.Debugf("Total records to be created: %d (%+v)", len(toCreate), toCreate)

	// get all the zones we're handling
	zones, err := p.client.GetMatchingZones(p.domainFilter)
	if err != nil {
		return fmt.Errorf("ApplyChanges: %s", err)
	}
	log.Debugf("Matching zones: %s", zones)

	// now assign operations to each zone
	zoneDeletions := map[string]([]*endpoint.Endpoint){}
	zoneCreations := map[string]([]*endpoint.Endpoint){}

	// init all zones with no operation
	for zone := range zones {
		zoneDeletions[zone] = []*endpoint.Endpoint{}
		zoneCreations[zone] = []*endpoint.Endpoint{}
	}

	// determine which zone to use.
	// We always operate in the most specific zone, eg
	// if we're managing c.d and b.c.d and the operation
	// is about a.b.c.d, we do it in the b.c.d zone

	for _, endpoint := range toDelete {
		zone, err := pickZone(endpoint.DNSName, zones)
		if err != nil {
			return fmt.Errorf("ApplyChanges: %s", err)
		}
		log.Debugf("Chosen zone %s for deletion of %s/%s", zone, endpoint.DNSName, endpoint.RecordType)
		zoneDeletions[zone] = append(zoneDeletions[zone], endpoint)
	}
	for _, endpoint := range toCreate {
		zone, err := pickZone(endpoint.DNSName, zones)
		if err != nil {
			return fmt.Errorf("ApplyChanges: %s", err)
		}
		log.Debugf("Chosen zone %s for creation of %s/%s", zone, endpoint.DNSName, endpoint.RecordType)
		zoneCreations[zone] = append(zoneCreations[zone], endpoint)
	}

	for zone, zoneData := range zones {
		// do deletions first
		if len(zoneDeletions[zone]) > 0 {
			log.Infof("Zone %s: %d deletions", zone, len(zoneDeletions[zone]))
			err = p.client.DeleteRecords(zone, zoneData, zoneDeletions[zone])
			if err != nil {
				return fmt.Errorf("ApplyChanges: %s", err)
			}
		}
		if len(zoneCreations[zone]) > 0 {
			log.Infof("Zone %s: %d creations", zone, len(zoneCreations[zone]))
			err = p.client.CreateRecords(zone, zoneData, zoneCreations[zone])
			if err != nil {
				return fmt.Errorf("ApplyChanges: %s", err)
			}
		}

	}
	return nil
}

// remove each part of the label starting from the left
// until we find a zone that we manage
func pickZone(dnsName string, zones map[string]*common.ZoneData) (string, error) {

	origName := dnsName
	expr := regexp.MustCompile(`^[^.]*\.?`)

	for dnsName != "" {
		if _, ok := zones[dnsName]; ok {
			return dnsName, nil
		}
		// remove first label from name and repeat
		dnsName = expr.ReplaceAllString(dnsName, "")
	}
	return "", fmt.Errorf("pickZone: cannot find zone for name '%s'", origName)
}
