package common

import (
	"fmt"
	"reflect"
	"regexp"

	"sigs.k8s.io/external-dns/endpoint"
)

// "github.com/go-test/deep"

type ZoneData struct {
	TargetLink      string
	HostedDnsZoneId string
}

func (z *ZoneData) String() string {
	return fmt.Sprintf("targetLink: %s, hostedDnsZoneId: %s", z.TargetLink, z.HostedDnsZoneId)
}

type ZoneInfo struct {
	Endpoints []*endpoint.Endpoint
	ZoneData  *ZoneData
}

// utilities
func CreateDomainFilter(regexDomainFilter string, regexDomainExclude string, listDomainFilter []string, listDomainFilterExclude []string) *endpoint.DomainFilter {
	domainFilter := endpoint.DomainFilter{}

	if regexDomainFilter != "" {
		domainFilter = endpoint.NewRegexDomainFilter(
			regexp.MustCompile(regexDomainFilter),
			regexp.MustCompile(regexDomainExclude),
		)
	} else {
		domainFilter = endpoint.NewDomainFilterWithExclusions(listDomainFilter, listDomainFilterExclude)
	}
	return &domainFilter

}

func ExpandRecords(eps []*endpoint.Endpoint) []*endpoint.Endpoint {

	//log.Infof("Must expand: %+v", eps)

	records := []*endpoint.Endpoint{}
	for _, ep := range eps {
		for _, target := range ep.Targets {
			record := endpoint.NewEndpointWithTTL(ep.DNSName, ep.RecordType, ep.RecordTTL, target)
			records = append(records, record)
		}
	}
	//log.Infof("After expansion: %+v", records)
	return records
}

// compare two lists of endpoints
func SameEndpoints(eps1 []*endpoint.Endpoint, eps2 []*endpoint.Endpoint) bool {

	if len(eps1) != len(eps2) {
		return false
	}

	for _, ep1 := range eps1 {

		found := false
		// look for ep1 inside eps2
		for _, ep2 := range eps2 {
			//diff := deep.Equal(ep1, ep2)
			//if diff != nil {
			//	log.Info(diff)
			//}

			if reflect.DeepEqual(ep1, ep2) {
				found = true
				break
			}
		}
		if found == false {
			//log.Infof("Record %s not found inside %+v", ep1, eps2)
			return false
		}
	}
	return true
}
