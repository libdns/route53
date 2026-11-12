package route53 //nolint:testpackage // Testing internal functions

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	r53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/libdns/libdns"
)

const (
	fakeZone  = "example.com."
	fakeXMLNS = "https://route53.amazonaws.com/doc/2013-04-01/"
)

// fakeRecordSet is the wire form of a Route53 ResourceRecordSet.
type fakeRecordSet struct {
	Name    string       `xml:"Name"`
	Type    string       `xml:"Type"`
	TTL     int64        `xml:"TTL"`
	Records []fakeRecord `xml:"ResourceRecords>ResourceRecord"`
}

type fakeRecord struct {
	Value string `xml:"Value"`
}

func newFakeRecordSet(name, recordType string, ttl int64, values ...string) fakeRecordSet {
	set := fakeRecordSet{Name: name, Type: recordType, TTL: ttl}
	for _, value := range values {
		set.Records = append(set.Records, fakeRecord{Value: value})
	}
	return set
}

func (s fakeRecordSet) String() string {
	values := make([]string, 0, len(s.Records))
	for _, record := range s.Records {
		values = append(values, record.Value)
	}
	return fmt.Sprintf("%s %s %d %s", s.Name, s.Type, s.TTL, strings.Join(values, ","))
}

type fakeListResponse struct {
	XMLName xml.Name        `xml:"https://route53.amazonaws.com/doc/2013-04-01/ ListResourceRecordSetsResponse"`
	Sets    []fakeRecordSet `xml:"ResourceRecordSets>ResourceRecordSet"`
}

type fakeChangeRequest struct {
	Changes []struct {
		Action string        `xml:"Action"`
		Set    fakeRecordSet `xml:"ResourceRecordSet"`
	} `xml:"ChangeBatch>Changes>Change"`
}

// fakeRoute53 is an in-memory stand-in for the two Route53 API calls the
// provider makes against a hosted zone: ListResourceRecordSets (GET) and
// ChangeResourceRecordSets (POST). Like the real service, DELETE only
// succeeds when the TTL and values match the stored record set exactly.
type fakeRoute53 struct {
	mu      sync.Mutex
	sets    []fakeRecordSet
	lists   int
	changes []string
}

func (f *fakeRoute53) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "text/xml")
	switch r.Method {
	case http.MethodGet:
		f.lists++
		_ = xml.NewEncoder(w).Encode(fakeListResponse{Sets: f.sets})
	case http.MethodPost:
		var req fakeChangeRequest
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
			f.reject(w, err.Error())
			return
		}
		for _, change := range req.Changes {
			if msg := f.apply(change.Action, change.Set); msg != "" {
				f.reject(w, msg)
				return
			}
		}
		fmt.Fprintf(w, `<ChangeResourceRecordSetsResponse xmlns="%s"><ChangeInfo><Id>/change/C1</Id>`+
			`<Status>INSYNC</Status><SubmittedAt>2026-01-01T00:00:00Z</SubmittedAt></ChangeInfo>`+
			`</ChangeResourceRecordSetsResponse>`, fakeXMLNS)
	default:
		f.reject(w, "unexpected method "+r.Method)
	}
}

// apply performs one change and returns a non-empty message if it is invalid.
func (f *fakeRoute53) apply(action string, set fakeRecordSet) string {
	f.changes = append(f.changes, fmt.Sprintf("%s %s %s", action, set.Name, set.Type))

	idx := slices.IndexFunc(f.sets, func(s fakeRecordSet) bool {
		return s.Name == set.Name && s.Type == set.Type
	})
	switch action {
	case "DELETE":
		if idx < 0 || f.sets[idx].String() != set.String() {
			return "DELETE does not match the stored record set: " + set.String()
		}
		f.sets = slices.Delete(f.sets, idx, idx+1)
	case "UPSERT":
		if idx < 0 {
			f.sets = append(f.sets, set)
		} else {
			f.sets[idx] = set
		}
	default:
		return "unexpected action " + action
	}
	return ""
}

func (f *fakeRoute53) reject(w http.ResponseWriter, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<ErrorResponse xmlns="%s"><Error><Type>Sender</Type><Code>InvalidChangeBatch</Code>`+
		`<Message>%s</Message></Error><RequestId>test</RequestId></ErrorResponse>`, fakeXMLNS, msg)
}

func (f *fakeRoute53) state() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	state := make([]string, 0, len(f.sets))
	for _, set := range f.sets {
		state = append(state, set.String())
	}
	slices.Sort(state)
	return state
}

// newFakeProvider returns a Provider wired to a fake Route53 server holding
// a small zone, plus the fake itself for inspection.
func newFakeProvider(t *testing.T) (*Provider, *fakeRoute53) {
	t.Helper()

	fake := &fakeRoute53{sets: []fakeRecordSet{
		newFakeRecordSet("example.com.", "A", 300, "192.0.2.10"),
		newFakeRecordSet("example.com.", "NS", 172800, "ns-1.example.net."),
		newFakeRecordSet("example.com.", "SOA", 900,
			"ns-1.example.net. hostmaster.example.com. 1 7200 900 1209600 86400"),
		newFakeRecordSet("other.example.com.", "A", 300, "192.0.2.9"),
		newFakeRecordSet("www.example.com.", "A", 300, "192.0.2.1", "192.0.2.2"),
		newFakeRecordSet("www.example.com.", "AAAA", 300, "2001:db8::1"),
		newFakeRecordSet("www.example.com.", "TXT", 300, `"hello"`),
	}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	provider := &Provider{HostedZoneID: "ZTEST"}
	// Mark initialization as done so init does not replace the fake client
	// with one built from the ambient AWS configuration.
	provider.initOnce.Do(func() {})
	provider.client = r53.New(r53.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(server.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	})

	return provider, fake
}

func TestDeleteRecords(t *testing.T) {
	const (
		apexA    = "example.com. A 300 192.0.2.10"
		apexNS   = "example.com. NS 172800 ns-1.example.net."
		apexSOA  = "example.com. SOA 900 ns-1.example.net. hostmaster.example.com. 1 7200 900 1209600 86400"
		otherA   = "other.example.com. A 300 192.0.2.9"
		wwwA     = "www.example.com. A 300 192.0.2.1,192.0.2.2"
		wwwAAAA  = "www.example.com. AAAA 300 2001:db8::1"
		wwwTXT   = `www.example.com. TXT 300 "hello"`
		wwwAOnly = "www.example.com. A 300 192.0.2.2"
	)

	cases := []struct {
		name        string
		input       []libdns.Record
		wantDeleted []string
		wantState   []string
		wantChanges []string
		wantLists   int
	}{
		{
			name:        "exact value removes one value from the set",
			input:       []libdns.Record{libdns.RR{Name: "www", Type: "A", Data: "192.0.2.1"}},
			wantDeleted: []string{"www A 192.0.2.1"},
			wantState:   []string{apexA, apexNS, apexSOA, otherA, wwwAOnly, wwwAAAA, wwwTXT},
			wantChanges: []string{"UPSERT www.example.com. A"},
			wantLists:   1,
		},
		{
			name:        "empty value deletes every value of that type",
			input:       []libdns.Record{libdns.RR{Name: "www", Type: "A"}},
			wantDeleted: []string{"www A 192.0.2.1", "www A 192.0.2.2"},
			wantState:   []string{apexA, apexNS, apexSOA, otherA, wwwAAAA, wwwTXT},
			wantChanges: []string{"DELETE www.example.com. A"},
			wantLists:   1,
		},
		{
			name:        "typed record with zero value deletes every value of that type",
			input:       []libdns.Record{libdns.Address{Name: "www"}},
			wantDeleted: []string{"www A 192.0.2.1", "www A 192.0.2.2"},
			wantState:   []string{apexA, apexNS, apexSOA, otherA, wwwAAAA, wwwTXT},
			wantChanges: []string{"DELETE www.example.com. A"},
			wantLists:   1,
		},
		{
			name:        "empty type deletes the value from whichever set holds it",
			input:       []libdns.Record{libdns.RR{Name: "www", Data: "192.0.2.1"}},
			wantDeleted: []string{"www A 192.0.2.1"},
			wantState:   []string{apexA, apexNS, apexSOA, otherA, wwwAOnly, wwwAAAA, wwwTXT},
			wantChanges: []string{"UPSERT www.example.com. A"},
			wantLists:   4, // discovery, then one read per set at the name
		},
		{
			name:        "empty type and value deletes everything at the name",
			input:       []libdns.Record{libdns.RR{Name: "www"}},
			wantDeleted: []string{"www A 192.0.2.1", "www A 192.0.2.2", "www AAAA 2001:db8::1", "www TXT hello"},
			wantState:   []string{apexA, apexNS, apexSOA, otherA},
			wantChanges: []string{
				"DELETE www.example.com. A",
				"DELETE www.example.com. AAAA",
				"DELETE www.example.com. TXT",
			},
			wantLists: 4,
		},
		{
			name:        "empty type at the apex leaves the zone's SOA and NS alone",
			input:       []libdns.Record{libdns.RR{Name: "@"}},
			wantDeleted: []string{"@ A 192.0.2.10"},
			wantState:   []string{apexNS, apexSOA, otherA, wwwA, wwwAAAA, wwwTXT},
			wantChanges: []string{"DELETE example.com. A"},
			wantLists:   2,
		},
		{
			name:        "TTL is not compared",
			input:       []libdns.Record{libdns.RR{Name: "other", Type: "A", TTL: time.Minute, Data: "192.0.2.9"}},
			wantDeleted: []string{"other A 192.0.2.9"},
			wantState:   []string{apexA, apexNS, apexSOA, wwwA, wwwAAAA, wwwTXT},
			wantChanges: []string{"DELETE other.example.com. A"},
			wantLists:   1,
		},
		{
			name: "records that do not exist are ignored without writing",
			input: []libdns.Record{
				libdns.RR{Name: "www", Type: "A", Data: "203.0.113.1"},
				libdns.RR{Name: "www", Type: "MX"},
				libdns.RR{Name: "missing"},
			},
			wantState: []string{apexA, apexNS, apexSOA, otherA, wwwA, wwwAAAA, wwwTXT},
			wantLists: 3, // discovery, then one read per typed input
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			provider, fake := newFakeProvider(t)

			deleted, err := provider.DeleteRecords(t.Context(), fakeZone, c.input)
			if err != nil {
				t.Fatalf("DeleteRecords: %v", err)
			}

			gotDeleted := make([]string, 0, len(deleted))
			for _, record := range deleted {
				rr := record.RR()
				gotDeleted = append(gotDeleted, fmt.Sprintf("%s %s %s", rr.Name, rr.Type, rr.Data))
			}
			slices.Sort(gotDeleted)
			if !slices.Equal(gotDeleted, c.wantDeleted) {
				t.Errorf("deleted records:\n got %q\nwant %q", gotDeleted, c.wantDeleted)
			}

			if gotState := fake.state(); !slices.Equal(gotState, c.wantState) {
				t.Errorf("zone after delete:\n got %q\nwant %q", gotState, c.wantState)
			}

			gotChanges := slices.Sorted(slices.Values(fake.changes))
			if !slices.Equal(gotChanges, c.wantChanges) {
				t.Errorf("changes sent:\n got %q\nwant %q", gotChanges, c.wantChanges)
			}

			if fake.lists != c.wantLists {
				t.Errorf("zone listings: got %d, want %d", fake.lists, c.wantLists)
			}
		})
	}
}
