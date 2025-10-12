package route53_libdnstest_test

import (
	"context"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"
	"github.com/libdns/libdns/libdnstest"
	"github.com/libdns/route53"
)

func TestRoute53Provider(t *testing.T) {
	// get credentials from environment
	accessKeyId := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	region := os.Getenv("AWS_REGION")
	profile := os.Getenv("AWS_PROFILE")
	testZone := os.Getenv("ROUTE53_TEST_ZONE")

	// check required environment variables
	if testZone == "" {
		t.Skip("Skipping Route53 provider tests: ROUTE53_TEST_ZONE environment variable must be set")
	}

	if !strings.HasSuffix(testZone, ".") {
		t.Fatal("We expect the test zone to have trailing dot")
	}

	// create provider with available credentials
	provider := &route53.Provider{
		Region:          region,
		Profile:         profile,
		AccessKeyId:     accessKeyId,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}

	suite := libdnstest.NewTestSuite(libdnstest.WrapNoZoneLister(provider), testZone)
	suite.RunTests(t)
}

func TestSkipRoute53SyncOnDelete_Performance(t *testing.T) {
	// get credentials from environment
	accessKeyId := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	region := os.Getenv("AWS_REGION")
	profile := os.Getenv("AWS_PROFILE")
	testZone := os.Getenv("ROUTE53_TEST_ZONE")

	// check required environment variables
	if testZone == "" {
		t.Skip("Skipping Route53 provider tests: ROUTE53_TEST_ZONE environment variable must be set")
	}

	if !strings.HasSuffix(testZone, ".") {
		t.Fatal("We expect the test zone to have trailing dot")
	}

	// create provider with waiting enabled but skip on delete operations
	provider := &route53.Provider{
		Region:                  region,
		Profile:                 profile,
		AccessKeyId:             accessKeyId,
		SecretAccessKey:         secretAccessKey,
		SessionToken:            sessionToken,
		WaitForRoute53Sync:      true,
		SkipRoute53SyncOnDelete: true,
	}

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

	// verify that delete was significantly faster than append
	// append should wait for sync (typically 5-60 seconds)
	// delete should skip sync (typically <2 seconds)
	t.Logf("Performance comparison: append=%v, delete=%v, ratio=%.2fx",
		appendDuration, deleteDuration, float64(appendDuration)/float64(deleteDuration))

	if deleteDuration >= appendDuration {
		t.Errorf("Delete operation took longer than append, expected delete to be faster due to SkipRoute53SyncOnDelete")
	}
}
