package main

import (
	"os"
	"strings"
	"testing"

	"github.com/libdns/libdns/libdnstest"
	"github.com/libdns/route53"
)

func TestRoute53Provider(t *testing.T) {
	// Get credentials from environment
	accessKeyId := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	region := os.Getenv("AWS_REGION")
	profile := os.Getenv("AWS_PROFILE")
	testZone := os.Getenv("ROUTE53_TEST_ZONE")

	// Check required environment variables
	if testZone == "" {
		t.Skip("Skipping Route53 provider tests: ROUTE53_TEST_ZONE environment variable must be set")
	}

	if !strings.HasSuffix(testZone, ".") {
		t.Fatal("We expect the test zone to have trailing dot")
	}

	// Create provider with available credentials
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