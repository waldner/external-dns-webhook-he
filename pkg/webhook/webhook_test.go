package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/waldner/external-dns-webhook-he/pkg/common"
	"github.com/waldner/external-dns-webhook-he/pkg/config"
	"github.com/waldner/external-dns-webhook-he/pkg/provider"
	"sigs.k8s.io/external-dns/endpoint"

	log "github.com/sirupsen/logrus"
)

func TestWebHook(t *testing.T) {
	for i, testCase := range common.TestCases {
		log.Infof("Running testcase %d", i)
		runSingleTest(testCase, t)
	}
}

func runSingleTest(testCase *common.TestCase, t *testing.T) {

	domainFilter := common.CreateDomainFilter(testCase.IncludeRegex, testCase.ExcludeRegex, testCase.IncludeList, testCase.ExcludeList)
	config := config.Config{}
	provider := provider.NewMockProvider(&config, domainFilter)

	hook, err := NewWebhook(provider)
	if err != nil {
		t.Errorf("Failure creating webHook: %s", err)
	}

	testNegotiate(t, hook, provider)
	testRecords(t, hook, provider)
	testAdjustEndpoints(t, hook, testCase)
	testApplyChanges(t, hook, testCase)

}

func testNegotiate(t *testing.T, hook *Webhook, provider *provider.Provider) {

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(hook.Negotiate)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusNotAcceptable {
		t.Errorf("/ handler returned wrong status code: got %d want %d", status, http.StatusNotAcceptable)
	}

	rr = httptest.NewRecorder()
	req.Header.Set("Accept", "foobar")
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusUnsupportedMediaType {
		t.Errorf("/ handler returned wrong status code: got %d want %d", status, http.StatusUnsupportedMediaType)
	}
	rr = httptest.NewRecorder()
	req.Header.Set("Accept", contentTypeValue)
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("/ handler returned wrong status code: got %d want %d", status, http.StatusOK)
	}

	wanted, _ := provider.DomainFilter().MarshalJSON()
	if string(wanted) != rr.Body.String() {
		t.Errorf("Unexpected response from /, want %s, got %s", wanted, rr.Body)
	}
}

func testRecords(t *testing.T, hook *Webhook, provider *provider.Provider) {

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(hook.Records)

	req, err := http.NewRequest("GET", "/records", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusNotAcceptable {
		t.Errorf("/records handler returned wrong status code: got %d want %d", status, http.StatusNotAcceptable)
	}

	rr = httptest.NewRecorder()
	req.Header.Set("Accept", "foobar")
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusUnsupportedMediaType {
		t.Errorf("/records handler returned wrong status code: got %d want %d", status, http.StatusUnsupportedMediaType)
	}

	rr = httptest.NewRecorder()
	req.Header.Set("Accept", contentTypeValue)
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("/records handler returned wrong status code: got %d want %d", status, http.StatusOK)
	}

	records := []*endpoint.Endpoint{}
	if err := json.NewDecoder(rr.Body).Decode(&records); err != nil {
		t.Errorf("cannot json-decode /records result: %s", err)
	}

	for _, record := range records {
		if record.Labels == nil {
			record.Labels = map[string]string{}
		}
	}

	wanted := []*endpoint.Endpoint{}
	for _, zone := range matchingZones(provider.DomainFilter()) {
		wanted = append(wanted, common.ExpandRecords(common.TestData[zone].Endpoints)...)
	}
	if !common.SameEndpoints(wanted, records) {
		t.Errorf("/records: received record set %v differs from wanted %v", records, wanted)
	}

}

func testAdjustEndpoints(t *testing.T, hook *Webhook, testCase *common.TestCase) {

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(hook.AdjustEndpoints)

	bodyBuf := new(bytes.Buffer)
	json.NewEncoder(bodyBuf).Encode(testCase.AdjustEndpointsInput)
	req, err := http.NewRequest("POST", "/adjustendpoints", bodyBuf)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusNotAcceptable {
		t.Errorf("/adjustendpoints handler returned wrong status code: got %d want %d", status, http.StatusNotAcceptable)
	}

	rr = httptest.NewRecorder()
	req.Header.Set("Content-Type", "foobar")
	req.Header.Set("Accept", "foobar")
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusUnsupportedMediaType {
		t.Errorf("/adjustendpoints handler returned wrong status code: got %d want %d", status, http.StatusUnsupportedMediaType)
	}
	rr = httptest.NewRecorder()
	req.Header.Set("Content-Type", contentTypeValue)
	req.Header.Set("Accept", contentTypeValue)
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("/adjustendpoints handler returned wrong status code: got %d want %d", status, http.StatusOK)
	}

	records := []*endpoint.Endpoint{}
	if err := json.NewDecoder(rr.Body).Decode(&records); err != nil {
		t.Errorf("cannot json-decode /adjustendpoints result: %s", err)
	}

	for _, record := range records {
		if record.Labels == nil {
			record.Labels = map[string]string{}
		}
	}

	if !common.SameEndpoints(common.ExpandRecords(testCase.AdjustEndpointsInput), records) {
		t.Errorf("/adjustendpoints: received record set %v differs from wanted %v", records, common.ExpandRecords(testCase.AdjustEndpointsInput))
	}
}

func testApplyChanges(t *testing.T, hook *Webhook, testCase *common.TestCase) {

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(hook.ApplyChanges)

	bodyBuf := new(bytes.Buffer)
	json.NewEncoder(bodyBuf).Encode(testCase.ApplyChangesInput)
	req, err := http.NewRequest("POST", "/records", bodyBuf)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusNotAcceptable {
		t.Errorf("/records POST handler returned wrong status code: got %d want %d", status, http.StatusNotAcceptable)
	}

	rr = httptest.NewRecorder()
	req.Header.Set("Content-Type", "foobar")
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusUnsupportedMediaType {
		t.Errorf("/records POST handler returned wrong status code: got %d want %d", status, http.StatusUnsupportedMediaType)
	}
	rr = httptest.NewRecorder()
	req.Header.Set("Content-Type", contentTypeValue)
	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusNoContent {
		t.Errorf("/records POST handler returned wrong status code: got %d want %d", status, http.StatusNoContent)
	}
}

// utility
func matchingZones(domainFilter *endpoint.DomainFilter) []string {

	zones := []string{}

	for zone, _ := range common.TestData {
		if !domainFilter.Match(zone) {
			continue
		}
		zones = append(zones, zone)

	}
	return zones
}
