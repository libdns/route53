package route53_libdnstest_test

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libdns/libdns"
	"github.com/libdns/libdns/libdnstest"
	"github.com/libdns/route53"
)

// providerFromEnv builds a Provider from the standard AWS env vars used by
// these integration tests and reads ROUTE53_TEST_ZONE for the target zone.
// Skips the test if ROUTE53_TEST_ZONE is unset.
func providerFromEnv(t *testing.T) (*route53.Provider, string) {
	t.Helper()
	testZone := os.Getenv("ROUTE53_TEST_ZONE")
	if testZone == "" {
		t.Skip("Skipping Route53 provider tests: ROUTE53_TEST_ZONE environment variable must be set")
	}
	if !strings.HasSuffix(testZone, ".") {
		t.Fatal("We expect the test zone to have trailing dot")
	}
	return &route53.Provider{
		Region:          os.Getenv("AWS_REGION"),
		Profile:         os.Getenv("AWS_PROFILE"),
		AccessKeyId:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
	}, testZone
}

func TestRoute53Provider(t *testing.T) {
	provider, testZone := providerFromEnv(t)
	suite := libdnstest.NewTestSuite(libdnstest.WrapNoZoneLister(provider), testZone)
	suite.RunTests(t)
}

func TestSkipRoute53SyncOnDelete_Performance(t *testing.T) {
	provider, testZone := providerFromEnv(t)
	provider.WaitForRoute53Sync = true
	provider.SkipRoute53SyncOnDelete = true

	ctx := context.Background()
	testRecord := &libdns.Address{
		Name: "test-append-record-r53-sync",
		TTL:  300 * time.Second,
		IP:   netip.MustParseAddr("192.0.2.1"),
	}

	// clean up first - delete the record if it exists (fast because we skip sync on delete)
	t.Log("Cleaning up any existing test record...")
	_, _ = provider.DeleteRecords(ctx, testZone, []libdns.Record{testRecord})

	// append the record - this should take longer because we wait for sync
	t.Log("Appending record with WaitForRoute53Sync=true (should be slow)...")
	appendStart := time.Now()
	_, err := provider.AppendRecords(ctx, testZone, []libdns.Record{testRecord})
	appendDuration := time.Since(appendStart)
	if err != nil {
		t.Fatalf("Failed to append record: %v", err)
	}
	t.Logf("Append took %v", appendDuration)

	// delete the record - this should be fast because we skip sync on delete
	t.Log("Deleting record with SkipRoute53SyncOnDelete=true (should be fast)...")
	deleteStart := time.Now()
	_, err = provider.DeleteRecords(ctx, testZone, []libdns.Record{testRecord})
	deleteDuration := time.Since(deleteStart)
	if err != nil {
		t.Fatalf("Failed to delete record: %v", err)
	}
	t.Logf("Delete took %v", deleteDuration)

	t.Logf("Performance comparison: append=%v, delete=%v, ratio=%.2fx",
		appendDuration, deleteDuration, float64(appendDuration)/float64(deleteDuration))

	if deleteDuration >= appendDuration {
		t.Errorf("Delete operation took longer than append, expected delete to be faster " +
			"due to SkipRoute53SyncOnDelete")
	}
}

// TestAppendRecords_Concurrent reproduces the read-modify-write race in
// AppendRecords. Several goroutines each append a distinct TXT value to the
// same (name, type). With a correct provider all values must be present
// after. With the racy implementation, concurrent reads observe stale state
// and the later UPSERT clobbers the earlier — fewer values survive.
func TestAppendRecords_Concurrent(t *testing.T) {
	provider, testZone := providerFromEnv(t)

	ctx := context.Background()
	const count = 10
	name := "test-concurrent-append"

	records := make([]libdns.Record, count)
	for i := range count {
		records[i] = &libdns.TXT{
			Name: name,
			TTL:  60 * time.Second,
			Text: fmt.Sprintf("token-%d", i),
		}
	}

	// pre- and post-test cleanup
	t.Cleanup(func() {
		_, _ = provider.DeleteRecords(ctx, testZone, records)
	})
	_, _ = provider.DeleteRecords(ctx, testZone, records)

	// fire all appends as close to simultaneously as possible to maximize
	// the read-modify-write overlap window
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func(rec libdns.Record) {
			defer wg.Done()
			<-start
			if _, err := provider.AppendRecords(ctx, testZone, []libdns.Record{rec}); err != nil {
				errs <- err
			}
		}(records[i])
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Logf("AppendRecords reported error (may be expected under contention): %v", err)
	}

	got, err := provider.GetRecords(ctx, testZone)
	if err != nil {
		t.Fatalf("GetRecords failed: %v", err)
	}

	// getRecords returns relative names, so compare against the relative name.
	seen := make(map[string]bool)
	for _, r := range got {
		rr := r.RR()
		if rr.Name == name && rr.Type == "TXT" {
			seen[rr.Data] = true
		}
	}

	missing := make([]string, 0)
	for i := range count {
		token := fmt.Sprintf("token-%d", i)
		if !seen[token] {
			missing = append(missing, token)
		}
	}

	if len(missing) > 0 {
		t.Errorf("concurrent AppendRecords lost tokens: %d of %d survived; missing %v",
			len(seen), count, missing)
	}
}

// TestSetRecords_MultiValue verifies that a single SetRecords call with two
// values for the same (name, type) ends up with both values present. Today's
// implementation iterates and UPSERTs each record individually, so the second
// call replaces the first.
func TestSetRecords_MultiValue(t *testing.T) {
	provider, testZone := providerFromEnv(t)

	ctx := context.Background()
	name := "test-set-multi"
	ip1 := netip.MustParseAddr("192.0.2.10")
	ip2 := netip.MustParseAddr("192.0.2.11")
	records := []libdns.Record{
		&libdns.Address{Name: name, TTL: 60 * time.Second, IP: ip1},
		&libdns.Address{Name: name, TTL: 60 * time.Second, IP: ip2},
	}

	t.Cleanup(func() {
		_, _ = provider.DeleteRecords(ctx, testZone, records)
	})
	_, _ = provider.DeleteRecords(ctx, testZone, records)

	if _, err := provider.SetRecords(ctx, testZone, records); err != nil {
		t.Fatalf("SetRecords failed: %v", err)
	}

	got, err := provider.GetRecords(ctx, testZone)
	if err != nil {
		t.Fatalf("GetRecords failed: %v", err)
	}

	// getRecords returns relative names, so compare against the relative name.
	seen := make(map[string]bool)
	for _, r := range got {
		rr := r.RR()
		if rr.Name == name && rr.Type == "A" {
			seen[rr.Data] = true
		}
	}

	if !seen[ip1.String()] {
		t.Errorf("missing %s after multi-value SetRecords; got %v", ip1, seen)
	}
	if !seen[ip2.String()] {
		t.Errorf("missing %s after multi-value SetRecords; got %v", ip2, seen)
	}
}
