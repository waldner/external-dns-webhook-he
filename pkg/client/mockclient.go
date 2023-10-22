package client

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/waldner/external-dns-webhook-he/pkg/common"
	"github.com/waldner/external-dns-webhook-he/pkg/config"
	"sigs.k8s.io/external-dns/endpoint"
)

type MockClient struct {
	config         *config.Config
	zoneInfo       map[string]*common.ZoneInfo
	failMap        map[string]bool
	CreatedRecords []*endpoint.Endpoint
	DeletedRecords []*endpoint.Endpoint
}

func NewMockClient(config *config.Config) *MockClient {

	return &MockClient{
		config:         config,
		CreatedRecords: []*endpoint.Endpoint{},
		DeletedRecords: []*endpoint.Endpoint{},
	}
}

func (c *MockClient) SetFailure(failure string) {
	c.failMap = map[string]bool{}
	if failure != "" {
		c.failMap[failure] = true
	}
}

func (c *MockClient) DoLogin() error {
	if c.failMap["DoLogin"] {
		return fmt.Errorf("DoLogin error")
	}
	return nil
}
func (c *MockClient) DoLogout() error {
	if c.failMap["DoLogout"] {
		return fmt.Errorf("DoLogout error")
	}
	return nil
}

func (c *MockClient) GetMatchingZones(domainFilter *endpoint.DomainFilter) (map[string]*common.ZoneData, error) {

	if c.failMap["GetMatchingZones"] {
		return nil, fmt.Errorf("GetMatchingZones error")
	}

	zones := map[string]*common.ZoneData{}

	for zone, zoneInfo := range common.TestData {
		if domainFilter.Match(zone) {
			zones[zone] = zoneInfo.ZoneData
		}
	}

	return zones, nil
}

func (c *MockClient) GetZoneEndpoints(zone string, zoneData *common.ZoneData) ([]*endpoint.Endpoint, error) {

	if c.failMap["GetZoneEndpoints"] {
		return nil, fmt.Errorf("GetZoneEndpoint error")
	}

	if _, ok := common.TestData[zone]; !ok {
		return nil, fmt.Errorf("Zone %s not found", zone)
	}

	return common.ExpandRecords(common.TestData[zone].Endpoints), nil
}

func (c *MockClient) CreateRecords(zone string, zoneData *common.ZoneData, records []*endpoint.Endpoint) error {

	if c.failMap["CreateRecords"] {
		return fmt.Errorf("CreateRecords error")
	}

	//log.Infof("Must create records: %+v", records)
	for _, record := range common.ExpandRecords(records) {
		log.Infof("Creating record %s", record)
		c.CreatedRecords = append(c.CreatedRecords, record)
	}
	return nil
}

func (c *MockClient) DeleteRecords(zone string, zoneData *common.ZoneData, records []*endpoint.Endpoint) error {

	if c.failMap["DeleteRecords"] {
		return fmt.Errorf("DeleteRecords error")
	}
	for _, record := range common.ExpandRecords(records) {
		log.Infof("Deleting record %s", record)
		c.DeletedRecords = append(c.DeletedRecords, record)
	}

	return nil
}
