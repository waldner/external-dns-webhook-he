package provider

import (
	"fmt"
	"testing"

	"github.com/waldner/external-dns-webhook-he/pkg/client"
	"github.com/waldner/external-dns-webhook-he/pkg/common"
	"github.com/waldner/external-dns-webhook-he/pkg/config"
	"sigs.k8s.io/external-dns/endpoint"
)

func TestProvider(t *testing.T) {
	for i, testCase := range common.TestCases {
		fmt.Printf("Running testCase %d\n", i)
		runSingleTestCase(t, testCase)
	}
}

func runSingleTestCase(t *testing.T, testCase *common.TestCase) {

	provider := createProvider(testCase)

	provider.client.(*client.MockClient).SetFailure("GetMatchingZones")
	matchingZones, err := provider.client.GetMatchingZones(provider.domainFilter)
	if err == nil {
		t.Errorf("GetMatchingZones should have failed")
	}
	provider.client.(*client.MockClient).SetFailure("")

	matchingZones, err = provider.client.GetMatchingZones(provider.domainFilter)
	if err != nil {
		t.Errorf("GetMatchingZones should not have failed, but got: %s", err)
	}

	///////////////////// test Records
	records, err := provider.GetAllRecords()
	if err != nil {
		t.Errorf("GetAllRecords should not have failed, but got: %s", err)
	}
	wanted := []*endpoint.Endpoint{}
	for zone, _ := range matchingZones {
		wanted = append(wanted, common.ExpandRecords(common.TestData[zone].Endpoints)...)
	}
	if !common.SameEndpoints(wanted, records) {
		t.Errorf("GetAllRecords: received record set %v differs from wanted %v", records, wanted)
	}

	provider.client.(*client.MockClient).SetFailure("GetMatchingZones")
	records, err = provider.GetAllRecords()
	if err == nil {
		t.Errorf("GetAllRecords should have failed")
	}
	provider.client.(*client.MockClient).SetFailure("")

	////////////////////// test AdjustEndpoints
	records, err = provider.AdjustEndpoints(testCase.AdjustEndpointsInput)
	if err != nil {
		t.Errorf("AdjustEndpoints should not have failed, but got: %s", err)
	}
	wanted = testCase.AdjustEndpointsInput
	if !common.SameEndpoints(wanted, records) {
		t.Errorf("AdjustEndpoints: received record set %v differs from wanted %v", records, wanted)
	}

	/////////////////////// test ApplyChanges
	err = provider.ApplyChanges(testCase.ApplyChangesInput)
	if err != nil {
		t.Errorf("ApplyChanges should not have failed, but got: %s", err)
	}

	// in provider.client.createdRecords we should have all creations + updateNew
	// in provider.client.deletedRecords we should have all deletions + updateOld

	//log.Infof("ApplyChangesInput is %+v", testCase.applyChangesInput)

	wanted = append(common.ExpandRecords(testCase.ApplyChangesInput.Create), common.ExpandRecords(testCase.ApplyChangesInput.UpdateNew)...)
	if !common.SameEndpoints(wanted, provider.client.(*client.MockClient).CreatedRecords) {
		t.Errorf("AppplyChanges creation: Received record set %v differs from wanted %v", provider.client.(*client.MockClient).CreatedRecords, wanted)
	}

	wanted = append(common.ExpandRecords(testCase.ApplyChangesInput.Delete), common.ExpandRecords(testCase.ApplyChangesInput.UpdateOld)...)
	if !common.SameEndpoints(wanted, provider.client.(*client.MockClient).DeletedRecords) {
		t.Errorf("ApplyChanges deletion: Received record set %v differs from wanted %v", provider.client.(*client.MockClient).DeletedRecords, wanted)
	}

	provider.client.(*client.MockClient).SetFailure("CreateRecords")
	err = provider.ApplyChanges(testCase.ApplyChangesInput)
	if err == nil {
		t.Errorf("ApplyChanges creation should have failed")
	}
	provider.client.(*client.MockClient).SetFailure("DeleteRecords")
	err = provider.ApplyChanges(testCase.ApplyChangesInput)
	if err == nil {
		t.Errorf("ApplyChanges deletion should have failed")
	}
	provider.client.(*client.MockClient).SetFailure("")

}

func createProvider(testCase *common.TestCase) *Provider {

	domainFilter := common.CreateDomainFilter(testCase.IncludeRegex, testCase.ExcludeRegex, testCase.IncludeList, testCase.ExcludeList)
	config := config.Config{}
	return NewMockProvider(&config, domainFilter)
}
