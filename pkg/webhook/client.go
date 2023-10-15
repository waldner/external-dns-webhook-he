package webhook

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/antchfx/htmlquery"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type zoneData struct {
	targetLink      string
	hostedDnsZoneId string
}

const (
	recordIdTag           = "edns.xdb.me/he-record-id"
	successfulRemovalMsg  = ">Successfully removed record.<"
	successfulCreationMsg = ">Successfully added new record to %s<"
	successfulUpdateMsg   = ">Successfully updated record. <"
)

func (z *zoneData) String() string {
	return fmt.Sprintf("targetLink: %s, hostedDnsZoneId: %s", z.targetLink, z.hostedDnsZoneId)
}

func getAllRecords(h *heWebhook) ([]*endpoint.Endpoint, error) {

	body, err := doLogin(h)
	if err != nil {
		return nil, fmt.Errorf("getAllRecords: %s", err)
	}

	defer doLogout(h)

	zones, err := getMatchingZones(body, h)
	if err != nil {
		return nil, fmt.Errorf("getAllRecords: %s", err)
	}

	log.Debugf("Matching zones according to domain filter: %v", zones)

	allEndpoints := []*endpoint.Endpoint{}

	for zone, zoneData := range zones {
		body, err = getZonePage(zone, zoneData, h)
		if err != nil {
			log.Errorf("error getting zone page: %s", err)
			log.Warnf("skipping zone '%s'", zone)
			continue
		}
		endpoints, err := getZoneEndpoints(body, zone, zoneData, h)
		if err != nil {
			log.Errorf("error getting zone records: %s", err)
			log.Warnf("Skipping zone '%s'", zone)
			continue
		}

		/*
			// TESTING
			for _, endpoint := range endpoints {
				if endpoint.RecordType == "TXT" {
					log.Debugf("Removing outer quotes for record %s", endpoint)
					endpoint.Targets[0] = removeOuterQuotes(endpoint.Targets[0])
					log.Debugf("Result is %s", endpoint)
				}
			}
		*/
		allEndpoints = append(allEndpoints, endpoints...)
	}

	return allEndpoints, nil

}

func getMatchingZones(body string, h *heWebhook) (map[string]*zoneData, error) {

	log.Infof("Getting matching domain list")

	tree, err := htmlquery.Parse(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("getMatchingZones: error parsing response body: %s", err)
	}

	zones := map[string]*zoneData{}

	// look for wanted zones
	for _, tr := range htmlquery.Find(tree, "//table[@id='domains_table']/tbody/tr") {
		z := htmlquery.InnerText(htmlquery.Find(tr, "./td[3]/span")[0])
		if !h.Conf.DomainFilter.Match(z) {
			continue
		}

		href := htmlquery.SelectAttr(htmlquery.Find(tr, "./td[2]/img")[0], "onclick")
		m := regexp.MustCompile(`^javascript:document\.location\.href='(.*)'$`)
		targetLink := m.ReplaceAllString(href, "$1")
		m = regexp.MustCompile(`.*hosted_dns_zoneid=(\d+).*`)
		hostedDnsZoneId := m.ReplaceAllString(targetLink, "$1")
		zones[z] = &zoneData{
			targetLink:      targetLink,
			hostedDnsZoneId: hostedDnsZoneId,
		}
	}

	return zones, nil
}

func adjustEndpoints(endpoints []*endpoint.Endpoint) []*endpoint.Endpoint {
	return endpoints
}

func isSameRecord(r1 *endpoint.Endpoint, r2 *endpoint.Endpoint) bool {

	if r1.DNSName == r2.DNSName &&
		r1.RecordType == r2.RecordType &&
		r1.Targets[0] == r2.Targets[0] {
		return true
	}

	return false
}

func deleteRecord(h *heWebhook, zone string, zoneData *zoneData, existingRecords []*endpoint.Endpoint, record *endpoint.Endpoint) error {

	//https://dns.he.net/?hosted_dns_zoneid=999999&menu=edit_zone&hosted_dns_editzone

	log.Infof("Deleting record: %s", record)

	// iterate to find the record ID
	recordId := ""
	for _, existingRecord := range existingRecords {
		if isSameRecord(existingRecord, record) {
			recordId, _ = existingRecord.GetProviderSpecificProperty(recordIdTag)
			break
		}
	}

	if recordId == "" {
		log.Warnf("Record %v not found, returning", record)
		return nil
	}

	postData := url.Values{}
	postData.Set("hosted_dns_zoneid", zoneData.hostedDnsZoneId)
	postData.Set("hosted_dns_recordid", recordId)
	postData.Set("menu", "edit_zone")
	postData.Set("hosted_dns_delconfirm", "delete")
	postData.Set("hosted_dns_editzone", "1")
	postData.Set("hosted_dns_delrecord", "1")

	body, response, err := postPage(h.Conf.Url+"/index.cgi", h, &postData)
	if err != nil {
		return fmt.Errorf("deleteRecord: %s", err)
	}

	// check that the HTTP code is correct
	if response.StatusCode != 200 {
		return fmt.Errorf("deleteRecord: got invalid status code %s", response.StatusCode)
	}

	// check that we're on the right page: there should be a ">Successfully removed record.<" message
	check := checkInPage(body, successfulRemovalMsg)
	if check == false {
		return fmt.Errorf("deleteRecord: cannot find the successful deletion message in page")
	}

	log.Infof("Successfully deleted record")
	return nil
}

func createRecord(h *heWebhook, zone string, zoneData *zoneData, existingRecords []*endpoint.Endpoint, record *endpoint.Endpoint) error {

	log.Infof("Creating record %s", record)

	postData := url.Values{}
	postData.Set("account", "")
	postData.Set("menu", "edit_zone")
	postData.Set("Type", record.RecordType)
	postData.Set("hosted_dns_zoneid", zoneData.hostedDnsZoneId)
	postData.Set("hosted_dns_recordid", "")
	postData.Set("hosted_dns_editzone", "1")
	postData.Set("Priority", "")
	postData.Set("Name", record.DNSName)
	postData.Set("Content", record.Targets[0])
	// TTL is always 0, so set it to 300
	postData.Set("TTL", "300") //strconv.FormatInt(int64(record.RecordTTL), 10))
	postData.Set("hosted_dns_editrecord", "Submit")

	body, response, err := postPage(h.Conf.Url+"/index.cgi", h, &postData)
	if err != nil {
		return fmt.Errorf("createRecord: %s", err)
	}

	// check also that the HTTP code is correct
	if response.StatusCode != 200 {
		return fmt.Errorf("createRecord: got invalid status code after creation/update of record %s: %v", record, response.StatusCode)
	}

	// check that we're on the right page: there should be a ">Successfully added new record to {domain}<" message
	// or, if it was an update, a "Successfully updated record" message

	check := checkInPage(body, fmt.Sprintf(successfulCreationMsg, zone))

	if check == false {
		return fmt.Errorf("createRecord: cannot find the expected creation message in page")
	}

	log.Infof("Successfully created record")

	return nil
}

// we have already determined the zone where we create or delete the records
func deleteRecords(h *heWebhook, zone string, zoneData *zoneData, deletions []*endpoint.Endpoint) error {

	// go to the zone page, then do all deletions
	body, err := getZonePage(zone, zoneData, h)
	if err != nil {
		return fmt.Errorf("deleteRecords: %s", err)
	}

	// read all existing records in page
	existingRecords, err := getZoneEndpoints(body, zone, zoneData, h)
	if err != nil {
		return fmt.Errorf("deleteRecords: %s", err)
	}

	for _, deletion := range deletions {
		err = deleteRecord(h, zone, zoneData, existingRecords, deletion)
		if err != nil {
			return fmt.Errorf("deleteRecords: %s", err)
		}
	}
	return nil
}

func createRecords(h *heWebhook, zone string, zoneData *zoneData, creations []*endpoint.Endpoint) error {

	// go to the zone page, then do all creations
	body, err := getZonePage(zone, zoneData, h)
	if err != nil {
		return fmt.Errorf("createRecords: %s", err)
	}

	// read all existing records in page
	existingRecords, err := getZoneEndpoints(body, zone, zoneData, h)
	if err != nil {
		return fmt.Errorf("createRecords: %s", err)
	}

	for _, r := range creations {

		for _, target := range r.Targets {

			record := r
			record.Targets = []string{target}

			err = createRecord(h, zone, zoneData, existingRecords, record)
			if err != nil {
				return fmt.Errorf("createRecords: %s", err)
			}
		}
	}
	return nil
}

func applyChanges(changes *plan.Changes, h *heWebhook) error {

	log.Debugf("Changes requested: create: %d, updateOld: %d, updateNew: %d, delete: %d", len(changes.Create), len(changes.UpdateOld), len(changes.UpdateNew), len(changes.Delete))

	if len(changes.Create) == 0 && len(changes.UpdateOld) == 0 && len(changes.UpdateNew) == 0 && len(changes.Delete) == 0 {
		log.Debugf("applyChanges: Nothing to do, returning")
		return nil
	}

	// updates become delete + create
	// we should also group changes by zone, so
	// all changes related to a zone are applied together later

	body, err := doLogin(h)
	if err != nil {
		return fmt.Errorf("applyChanges: %s", err)
	}

	defer doLogout(h)

	// group requested changes into creations and deletions
	toDelete := append(changes.UpdateOld, changes.Delete...)
	toCreate := append(changes.UpdateNew, changes.Create...)

	log.Debugf("Total records to be deleted: %d (%+v)", len(toDelete), toDelete)
	log.Debugf("Total records to be created: %d (%+v)", len(toCreate), toCreate)

	// get all the zones we're handling
	zones, err := getMatchingZones(body, h)
	if err != nil {
		return fmt.Errorf("applyChanges: %s", err)
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
			return fmt.Errorf("applyChanges: %s", err)
		}
		log.Debugf("Chosen zone %s for deletion of %s, type %s", zone, endpoint.DNSName, endpoint.RecordType)
		zoneDeletions[zone] = append(zoneDeletions[zone], endpoint)
	}
	for _, endpoint := range toCreate {
		zone, err := pickZone(endpoint.DNSName, zones)
		if err != nil {
			return fmt.Errorf("applyChanges: %s", err)
		}
		log.Debugf("Chosen zone %s for creation of %s, type %s", zone, endpoint.DNSName, endpoint.RecordType)
		zoneCreations[zone] = append(zoneCreations[zone], endpoint)
	}

	for zone, zoneData := range zones {
		// do deletions first
		if len(zoneDeletions[zone]) > 0 {
			log.Infof("Zone %s: %d deletions", zone, len(zoneDeletions[zone]))
			err = deleteRecords(h, zone, zoneData, zoneDeletions[zone])
			if err != nil {
				return fmt.Errorf("applyChanges: %s", err)
			}
		}
		if len(zoneCreations[zone]) > 0 {
			log.Infof("Zone %s: %d creations", zone, len(zoneCreations[zone]))
			err = createRecords(h, zone, zoneData, zoneCreations[zone])
			if err != nil {
				return fmt.Errorf("applyChanges: %s", err)
			}
		}

	}
	return nil
}

// remove each part of the label starting from the left
// until we find a zone that we manage
func pickZone(dnsName string, zones map[string]*zoneData) (string, error) {

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
