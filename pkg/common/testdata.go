package common

import (
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

var TestData map[string]*ZoneInfo = map[string]*ZoneInfo{
	"foo.bar": &ZoneInfo{
		ZoneData: &ZoneData{}, // not used by the mock client
		Endpoints: []*endpoint.Endpoint{
			endpoint.NewEndpoint("a.foo.bar", "A", "1.1.1.1"),
			endpoint.NewEndpoint("b.foo.bar", "A", "1.1.1.3"),
			endpoint.NewEndpoint("z.foo.bar", "A", "1.1.1.4"),
			endpoint.NewEndpoint("z.foo.bar", "TXT", "foobar"),
		}},
	"foo.baz": &ZoneInfo{
		ZoneData: &ZoneData{},
		Endpoints: []*endpoint.Endpoint{
			endpoint.NewEndpoint("n1.foo.baz", "A", "192.168.1.1"),
			endpoint.NewEndpoint("hello.foo.baz", "A", "192.168.1.3"),
			endpoint.NewEndpoint("foo.baz", "A", "192.168.1.4"),
		}},
	"foo.zzz": &ZoneInfo{
		ZoneData: &ZoneData{},
		Endpoints: []*endpoint.Endpoint{
			endpoint.NewEndpoint("single.foo.zzz", "A", "172.16.100.199", "172.16.100.200"),
			endpoint.NewEndpoint("bbb.foo.zzz", "A", "172.17.100.199"),
		},
	},
}

type TestCase struct {
	IncludeList          []string
	ExcludeList          []string
	IncludeRegex         string
	ExcludeRegex         string
	AdjustEndpointsInput []*endpoint.Endpoint
	ApplyChangesInput    *plan.Changes
}

var TestCases []*TestCase = []*TestCase{
	&TestCase{
		IncludeList: []string{"foo.bar", "foo.baz"},
		AdjustEndpointsInput: []*endpoint.Endpoint{
			endpoint.NewEndpoint("aaa.foo.bar", "A", "10.1.1.1"),
			endpoint.NewEndpoint("bbb.foo.bar", "A", "10.9.1.99"),
			endpoint.NewEndpoint("ccc.foo.bar", "A", "10.10.1.78"),
			endpoint.NewEndpoint("ddd.foo.bar", "A", "10.8.1.56"),
			endpoint.NewEndpoint("eee.foo.bar", "A", "10.4.1.22"),
		},
		ApplyChangesInput: &plan.Changes{
			Create: []*endpoint.Endpoint{
				endpoint.NewEndpoint("aaa.foo.bar", "A", "10.1.1.1"),
			},
			Delete: []*endpoint.Endpoint{
				endpoint.NewEndpoint("aaa.foo.bar", "A", "10.1.1.1"),
			},
			UpdateOld: []*endpoint.Endpoint{
				endpoint.NewEndpointWithTTL("update.foo.baz", "A", 500, "1.1.1.1", "2.2.2.2"),
			},
			UpdateNew: []*endpoint.Endpoint{
				endpoint.NewEndpointWithTTL("update.foo.baz", "A", 1500, "3.3.3.3", "5.5.5.5"),
			},
		},
	},
	&TestCase{
		IncludeList: []string{"foo.zzz"},
		AdjustEndpointsInput: []*endpoint.Endpoint{
			endpoint.NewEndpoint("aaa.foo.zzz", "A", "10.1.1.1"),
			endpoint.NewEndpoint("bbb.foo.zzz", "A", "192.168.1.99"),
		},
		ApplyChangesInput: &plan.Changes{
			Create: []*endpoint.Endpoint{
				endpoint.NewEndpoint("aaa.foo.zzz", "A", "10.1.1.1"),
			},
			Delete: []*endpoint.Endpoint{
				endpoint.NewEndpoint("bbb.foo.zzz", "A", "10.1.1.1"),
			},
			UpdateOld: []*endpoint.Endpoint{
				endpoint.NewEndpointWithTTL("single.foo.zzz", "A", 500, "172.16.100.199", "172.16.100.200"),
			},
			UpdateNew: []*endpoint.Endpoint{
				endpoint.NewEndpointWithTTL("single.foo.zzz", "A", 1500, "172.16.100.199", "172.16.100.200"),
			},
		},
	},
}
