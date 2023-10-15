package webhook

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/antchfx/htmlquery"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
)

const (
	failedLoginMsg  = ">Incorrect</div>"
	managingZoneMsg = ">Managing zone: %s<"
)

func readBody(response *http.Response) (string, error) {

	b, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		return "", fmt.Errorf("readBody: error reading response body: %s", err)
	}
	return string(b), nil

}

func doLogin(h *heWebhook) (string, error) {

	// fetch initial page to get the cookie
	_, _, err := getPage(h.Conf.Url, h)
	if err != nil {
		return "", fmt.Errorf("doLogin: %s", err)
	}

	log.Debugf("Logging in as user '%s'", h.Conf.Username)
	postData := url.Values{}
	postData.Set("email", h.Conf.Username)
	postData.Set("pass", h.Conf.Password)
	postData.Set("submit", "Login!")

	body, _, err := postPage(h.Conf.Url, h, &postData)
	if err != nil {
		return "", fmt.Errorf("doLogin: %s", err)
	}

	check := checkInPage(body, failedLoginMsg)

	if check == true {
		return "", fmt.Errorf("doLogin: Login failed (invalid credentials?)")
	}

	return body, nil
}

func doLogout(h *heWebhook) error {
	log.Debugf("Logging out...")
	_, _, err := getPage(h.Conf.Url+"?action=logout", h) // TODO response
	return err
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

func getZonePage(zone string, zoneData *zoneData, h *heWebhook) (string, error) {
	url := h.Conf.Url + zoneData.targetLink
	body, response, err := getPage(url, h)
	if err != nil {
		return "", fmt.Errorf("getZonePage: %s", err)
	}

	if response.Statuscode != 200 {
		return "", fmt.Errorf("getZonePage: unexpected response status: %s", response.Status)
	}

	check := checkInPage(body, fmt.Sprintf(managingZoneMsg, zone))
	if check == false {
		return "", fmt.Errorf("getZonePage: Expected text not found in zone page")
	}
	return body, nil

}

func getPage(url string, h *heWebhook) (string, *http.Response, error) {

	log.Debugf("Navigating to page '%s'", url)
	response, err := h.Client.Get(url)
	if err != nil {
		return "", nil, fmt.Errorf("getPage: Error fetching page '%s': %s", url, err)
	}

	log.Debugf("Page '%s' response: status %s, headers %s", url, response.Status, response.Header)

	body, err := readBody(response)
	if err != nil {
		return "", nil, fmt.Errorf("getPage: %s", err)
	}
	//log.Debugf("Body is %s", body)
	return body, response, nil
}

func postPage(url string, h *heWebhook, postData *url.Values) (string, *http.Response, error) {

	log.Debugf("Posting data to page %s", url)

	response, err := h.Client.PostForm(url, *postData)
	if err != nil {
		return "", nil, fmt.Errorf("postPage: submission error: %s", err)
	}

	log.Debugf("Page %s response: status %s, headers %s", url, response.Status, response.Header)
	body, err := readBody(response)
	if err != nil {
		return "", nil, fmt.Errorf("postPage: %s", err)
	}

	//log.Debugf("Body is %s", body)
	return body, response, nil
}

func getZoneEndpoints(body string, zone string, zoneData *zoneData, h *heWebhook) ([]*endpoint.Endpoint, error) {

	log.Infof("Getting endpoints for zone %s", zone)
	log.Debugf("Zone data is %s", *zoneData)

	tree, err := htmlquery.Parse(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("getZoneEndpoints: parsing HTML body: %s", err)
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
		log.Debugf("Zone %s (%s): read record %s", zone, zoneData.hostedDnsZoneId, ep)
		endpoints = append(endpoints, ep)
	}

	return endpoints, nil

}
