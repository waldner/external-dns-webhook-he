package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/antchfx/htmlquery"
	log "github.com/sirupsen/logrus"
	"github.com/waldner/external-dns-webhook-he/pkg/common"
	"github.com/waldner/external-dns-webhook-he/pkg/config"
	"sigs.k8s.io/external-dns/endpoint"
)

type HEClient struct {
	config   *config.Config
	client   *http.Client
	lastBody string
}

const (
	recordIdTag           = "edns.xdb.me/he-record-id"
	successfulRemovalMsg  = ">Successfully removed record.<"
	successfulCreationMsg = ">Successfully added new record to %s<"
	successfulUpdateMsg   = ">Successfully updated record. <"
	failedLoginMsg        = ">Incorrect</div>"
	managingZoneMsg       = ">Managing zone: %s<"
)

func NewClient(config *config.Config) (*HEClient, error) {

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("NewClient: error creating cookie jar: %s", err)
	}

	client := &http.Client{
		Jar: jar,
	}

	return &HEClient{
		config,
		client,
		"",
	}, nil

}

func (c *HEClient) DoLogin() error {

	// fetch initial page to get the cookie
	_, err := c.getPage(c.config.Url)
	if err != nil {
		return fmt.Errorf("DoLogin: %s", err)
	}

	log.Debugf("Logging in as user '%s'", c.config.Username)
	postData := url.Values{}
	postData.Set("email", c.config.Username)
	postData.Set("pass", c.config.Password)
	postData.Set("submit", "Login!")

	_, err = c.postPage(c.config.Url, &postData)
	if err != nil {
		return fmt.Errorf("DoLogin: %s", err)
	}

	if checkInPage(c.lastBody, failedLoginMsg) {
		return fmt.Errorf("DoLogin: Login failed (invalid credentials?)")
	}

	return nil
}

func (c *HEClient) DoLogout() error {
	log.Debugf("Logging out...")
	_, err := c.getPage(c.config.Url + "?action=logout") // TODO response
	return err
}

func (c *HEClient) getZonePage(zone string, zoneData *common.ZoneData) error {
	url := c.config.Url + zoneData.TargetLink
	response, err := c.getPage(url)
	if err != nil {
		return fmt.Errorf("getZonePage: %s", err)
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("getZonePage: unexpected response status: %s", response.Status)
	}

	if !checkInPage(c.lastBody, fmt.Sprintf(managingZoneMsg, zone)) {
		return fmt.Errorf("getZonePage: Expected text not found in zone page")
	}

	return nil

}

func (c *HEClient) getPage(url string) (*http.Response, error) {

	log.Debugf("Navigating to page '%s'", url)
	response, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("getPage: Error fetching page '%s': %s", url, err)
	}

	log.Debugf("Page '%s' response: status %s, headers %s", url, response.Status, response.Header)

	body, err := readBody(response)
	if err != nil {
		return nil, fmt.Errorf("getPage: %s", err)
	}
	//log.Debugf("Body is %s", body)

	c.lastBody = body
	return response, nil
}

func (c *HEClient) postPage(url string, postData *url.Values) (*http.Response, error) {

	log.Debugf("Posting data to page %s", url)

	response, err := c.client.PostForm(url, *postData)
	if err != nil {
		return nil, fmt.Errorf("postPage: submission error: %s", err)
	}

	log.Debugf("Page %s response: status %s, headers %s", url, response.Status, response.Header)
	body, err := readBody(response)
	if err != nil {
		return nil, fmt.Errorf("postPage: %s", err)
	}

	//log.Debugf("Body is %s", body)
	c.lastBody = body
	return response, nil
}

func (c *HEClient) GetMatchingZones(domainFilter *endpoint.DomainFilter) (map[string]*common.ZoneData, error) {

	log.Infof("Getting matching domain list")

	tree, err := htmlquery.Parse(strings.NewReader(c.lastBody))
	if err != nil {
		return nil, fmt.Errorf("GetMatchingZones: error parsing body: %s", err)
	}

	zones := map[string]*common.ZoneData{}

	// look for wanted zones
	for _, tr := range htmlquery.Find(tree, "//table[@id='domains_table']/tbody/tr") {
		z := htmlquery.InnerText(htmlquery.Find(tr, "./td[3]/span")[0])
		if !domainFilter.Match(z) {
			continue
		}

		href := htmlquery.SelectAttr(htmlquery.Find(tr, "./td[2]/img")[0], "onclick")
		m := regexp.MustCompile(`^javascript:document\.location\.href='(.*)'$`)
		targetLink := m.ReplaceAllString(href, "$1")
		m = regexp.MustCompile(`.*hosted_dns_zoneid=(\d+).*`)
		hostedDnsZoneId := m.ReplaceAllString(targetLink, "$1")
		zones[z] = &common.ZoneData{
			TargetLink:      targetLink,
			HostedDnsZoneId: hostedDnsZoneId,
		}
	}

	return zones, nil
}

func (c *HEClient) GetZoneEndpoints(zone string, zoneData *common.ZoneData) ([]*endpoint.Endpoint, error) {

	log.Infof("Getting endpoints for zone %s", zone)
	log.Debugf("Zone data is %s", *zoneData)

	err := c.getZonePage(zone, zoneData)
	if err != nil {
		return nil, fmt.Errorf("GetZoneEndpoints: %s", err)
	}

	tree, err := htmlquery.Parse(strings.NewReader(c.lastBody))
	if err != nil {
		return nil, fmt.Errorf("GetZoneEndpoints: parsing HTML body: %s", err)
	}

	/*
					Typical tr structure (slightly reformatted):


				        <tr>
					                        <th class="hidden">Zone Id</th>
		                         			<th class="hidden">Record Id</th>
								<th style="width: 25px;">Name</th>
								<th style="width: 25px;">Type</th>
								<th style="width: 25px;">TTL</th>
								<th style="width: 25px;">Priority</th>
								<th style="width: 25px;">Data</th>
								<th style="width: 25px;">DDNS</th>
								<th style="width: 25px;">Delete</th>

				        </tr>



					<tr class="dns_tr" id="4819821031" title="Click to edit this item." onclick="editRow(this)">
						<td class="hidden">933183</td>
						<td class="hidden">4819821031</td>
						<td width="95%" class="dns_view">mytxt.mydomain.com</td>

						<!-- <td align="center" ><img src="/include/images/types/txt.gif" data="TXT" alt="TXT"/></td> -->
						<td align="center" ><span class="rrlabel TXT" data="TXT" alt="TXT" >TXT</span></td>
						<td align="left">7200</td>
						<td align="center">-</td>
						<td align="left" data="&quot;CONTENTS&quot;" onclick="event.cancelBubble=true; alert($(this).attr('data'));" title="Click to view entire contents." >&quot;CONTENTS&quot;</td>
						<td class="hidden">0</td>
						<td></td>
						<td align="center" class="dns_delete"  onclick="event.cancelBubble=true;deleteRecord('4819821031','mytxt.domain.com','TXT')" title="Click to delete this record.">
						<img src="/include/images/delete.png" alt="delete"/>
						</td>
				</tr>
	*/

	endpoints := []*endpoint.Endpoint{}

	// NOTE: the "tbody" isn't in the actual html, but since go's parser adds it,
	// we must include it in the xpath
	for _, tr := range htmlquery.Find(tree, "//div[@id='dns_main_content']/table/tbody/tr[@class='dns_tr' or @class='dns_tr_locked']") {

		recordId := htmlquery.InnerText(htmlquery.FindOne(tr, "./td[2]"))

		recordName := htmlquery.InnerText(htmlquery.FindOne(tr, "./td[3]"))
		td := htmlquery.FindOne(tr, "./td[4]/span")
		recordType := htmlquery.SelectAttr(td, "data")

		td = htmlquery.FindOne(tr, "./td[7]")
		recordData := htmlquery.SelectAttr(td, "data")

		recordTtl := htmlquery.InnerText(htmlquery.FindOne(tr, "./td[5]"))

		intTtl, err := strconv.Atoi(recordTtl)
		if err != nil {
			log.Warnf("Cannot parse TTL '%s' for record %s of type %s: %s, skipping", recordTtl, recordName, recordType, err)
			continue
		}

		ep := endpoint.NewEndpointWithTTL(recordName, recordType, endpoint.TTL(intTtl), recordData)
		ep = ep.WithProviderSpecific(recordIdTag, recordId)
		log.Debugf("Zone %s (%s): read record %s", zone, zoneData.HostedDnsZoneId, ep)
		endpoints = append(endpoints, ep)
	}

	return endpoints, nil
}

func readBody(response *http.Response) (string, error) {

	b, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		return "", fmt.Errorf("readBody: error reading response body: %s", err)
	}
	return string(b), nil

}

func (c *HEClient) CreateRecords(zone string, zoneData *common.ZoneData, records []*endpoint.Endpoint) error {

	// go to the zone page and read all existing records in page
	existingRecords, err := c.GetZoneEndpoints(zone, zoneData)
	if err != nil {
		return fmt.Errorf("CreateRecords: %s", err)
	}

	log.Infof("==== Start record creation ====")
	for _, record := range records {
		err = c.createRecord(zone, zoneData, existingRecords, record)
		if err != nil {
			return fmt.Errorf("CreateRecords: %s", err)
		}
	}
	log.Infof("==== End record creation ====")
	return nil
}

func (c *HEClient) createRecord(zone string, zoneData *common.ZoneData, existingRecords []*endpoint.Endpoint, record *endpoint.Endpoint) error {

	log.Infof("Creating record %s", record)

	postData := url.Values{}
	postData.Set("account", "")
	postData.Set("menu", "edit_zone")
	postData.Set("Type", record.RecordType)
	postData.Set("hosted_dns_zoneid", zoneData.HostedDnsZoneId)
	postData.Set("hosted_dns_recordid", "")
	postData.Set("hosted_dns_editzone", "1")
	postData.Set("Priority", "")
	postData.Set("Name", record.DNSName)
	postData.Set("Content", record.Targets[0])
	// TTL is always 0, so set it to 300
	postData.Set("TTL", "300") //strconv.FormatInt(int64(record.RecordTTL), 10))
	postData.Set("hosted_dns_editrecord", "Submit")

	response, err := c.postPage(c.config.Url+"/index.cgi", &postData)
	if err != nil {
		return fmt.Errorf("createRecord: %s", err)
	}

	// check also that the HTTP code is correct
	if response.StatusCode != 200 {
		return fmt.Errorf("createRecord: got invalid status code after creation/update of record %s: %v", record, response.StatusCode)
	}

	// check that we're on the right page: there should be a ">Successfully added new record to {domain}<" message
	// or, if it was an update, a "Successfully updated record" message

	if !checkInPage(c.lastBody, fmt.Sprintf(successfulCreationMsg, zone)) {
		return fmt.Errorf("createRecord: cannot find the expected creation message in page")
	}

	log.Infof("Successfully created record")

	return nil
}

// we have already determined the zone where we create or delete the records
func (c *HEClient) DeleteRecords(zone string, zoneData *common.ZoneData, records []*endpoint.Endpoint) error {

	// go to the zone page and read all existing records in page
	existingRecords, err := c.GetZoneEndpoints(zone, zoneData)
	if err != nil {
		return fmt.Errorf("deleteRecords: %s", err)
	}

	log.Infof("==== Start record deletion ====")
	for _, record := range records {
		err = c.deleteRecord(zone, zoneData, existingRecords, record)
		if err != nil {
			return fmt.Errorf("DeleteRecords: %s", err)
		}
	}
	log.Infof("==== End record deletion ====")
	return nil
}

func (c *HEClient) deleteRecord(zone string, zoneData *common.ZoneData, existingRecords []*endpoint.Endpoint, record *endpoint.Endpoint) error {

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
		log.Warnf("Record %s not found, nothing to do, returning", record)
		return nil
	}

	postData := url.Values{}
	postData.Set("hosted_dns_zoneid", zoneData.HostedDnsZoneId)
	postData.Set("hosted_dns_recordid", recordId)
	postData.Set("menu", "edit_zone")
	postData.Set("hosted_dns_delconfirm", "delete")
	postData.Set("hosted_dns_editzone", "1")
	postData.Set("hosted_dns_delrecord", "1")

	response, err := c.postPage(c.config.Url+"/index.cgi", &postData)
	if err != nil {
		return fmt.Errorf("deleteRecord: %s", err)
	}

	// check that the HTTP code is correct
	if response.StatusCode != 200 {
		return fmt.Errorf("deleteRecord: got invalid status code %d", response.StatusCode)
	}
	// check that we're on the right page: there should be a ">Successfully removed record.<" message
	if !checkInPage(c.lastBody, successfulRemovalMsg) {
		return fmt.Errorf("deleteRecord: cannot find the successful deletion message in page")
	}

	log.Infof("Successfully deleted record")
	return nil
}

// compares two records
func isSameRecord(r1 *endpoint.Endpoint, r2 *endpoint.Endpoint) bool {

	if r1.DNSName == r2.DNSName &&
		r1.RecordType == r2.RecordType &&
		r1.Targets[0] == r2.Targets[0] {
		return true
	}

	return false
}

// check that the page contains a given string
func checkInPage(body string, wantedMsg string) bool {
	if !strings.Contains(body, wantedMsg) {
		log.Debugf("cannot find message '%s' in page", wantedMsg)
		return false
	}
	log.Debugf("found message '%s' in page", wantedMsg)
	return true
}
