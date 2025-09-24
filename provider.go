package route53

import (
	"context"
	"strings"
	"time"

	r53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/libdns/libdns"
)

// Provider implements the libdns interfaces for Route53.
//
// By default, the provider loads the AWS configuration from the environment.
// To override these values, set the fields in the Provider struct.
type Provider struct {
	client *r53.Client

	// Region is the AWS Region to use. If not set, it will use AWS_REGION
	// environment variable.
	Region string `json:"region,omitempty"`

	// AWSProfile is the AWS Profile to use. If not set, it will use
	// AWS_PROFILE environment variable.
	//
	// Deprecated: Use Profile instead
	AWSProfile string `json:"aws_profile,omitempty"`

	// AWSProfile is the AWS Profile to use. If not set, it will use
	// AWS_PROFILE environment variable.
	Profile string `json:"profile,omitempty"`

	// AccessKeyId is the AWS Access Key ID to use. If not set, it will use
	// AWS_ACCESS_KEY_ID
	AccessKeyId string `json:"access_key_id,omitempty"`

	// SecretAccessKey is the AWS Secret Access Key to use. If not set, it will use
	// AWS_SECRET_ACCESS_KEY environment variable.
	SecretAccessKey string `json:"secret_access_key,omitempty"`

	// Token is the AWS Session Token to use. If not set, it will use
	// AWS_SESSION_TOKEN environment variable.
	//
	// Deprecated: Use SessionToken instead.
	Token string `json:"token,omitempty"`

	// SessionToken is the AWS Session Token to use. If not set, it will use
	// AWS_SESSION_TOKEN environment variable.
	SessionToken string `json:"session_token,omitempty"`

	// MaxRetries is the maximum number of retries to make when a request
	// fails. If not set, it will use 5 retries.
	MaxRetries int `json:"max_retries,omitempty"`

	// MaxWaitDur is the maximum amount of time in seconds to wait for a record
	// to be propagated. If not set, it will 1 minute.
	MaxWaitDur time.Duration `json:"max_wait_dur,omitempty"`

	// WaitForPropagation if set to true, it will wait for the record to be
	// propagated before returning.
	WaitForPropagation bool `json:"wait_for_propagation,omitempty"`

	// HostedZoneID is the ID of the hosted zone to use. If not set, it will
	// be discovered from the zone name.
	//
	// This option should contain only the ID; the "/hostedzone/" prefix
	// will be added automatically.
	HostedZoneID string `json:"hosted_zone_id,omitempty"`
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	p.init(ctx)

	zoneID, err := p.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	records, err := p.getRecords(ctx, zoneID, zone)
	if err != nil {
		return nil, err
	}

	return records, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.init(ctx)

	zoneID, err := p.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	// Group records by name+type since Route53 treats them as a single ResourceRecordSet
	recordSets := make(map[string][]libdns.Record)
	for _, record := range records {
		rr := record.RR()
		key := rr.Name + ":" + rr.Type
		recordSets[key] = append(recordSets[key], record)
	}

	var createdRecords []libdns.Record

	// Process each record set
	for _, recordGroup := range recordSets {
		if len(recordGroup) == 0 {
			continue
		}

		// For groups with only one record, use the simple create
		if len(recordGroup) == 1 {
			newRecord, err := p.createRecord(ctx, zoneID, recordGroup[0], zone)
			if err != nil {
				return nil, err
			}
			createdRecords = append(createdRecords, newRecord)
			continue
		}

		// For multiple records with same name+type, we need to create them all at once
		// or append to existing set if it already exists
		firstRecord := recordGroup[0]
		rr := firstRecord.RR()

		existingRecords, err := p.getRecords(ctx, zoneID, zone)
		if err != nil {
			return nil, err
		}

		var existingValues []libdns.Record
		for _, existing := range existingRecords {
			existingRR := existing.RR()
			if existingRR.Name == libdns.AbsoluteName(rr.Name, zone) && existingRR.Type == rr.Type {
				existingValues = append(existingValues, existing)
			}
		}

		allRecords := append(existingValues, recordGroup...)

		// Use UPSERT to set all values at once
		created, err := p.setRecordSet(ctx, zoneID, zone, rr.Name, rr.Type, allRecords)
		if err != nil {
			return nil, err
		}

		// Add only the new records to the result
		createdRecords = append(createdRecords, recordGroup...)
		_ = created // created contains all records, but we only return what was requested
	}

	return createdRecords, nil
}

// DeleteRecords deletes the records from the zone. If a record does not have an ID,
// it will be looked up. It returns the records that were deleted.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.init(ctx)

	zoneID, err := p.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	existingRecords, err := p.getRecords(ctx, zoneID, zone)
	if err != nil {
		return nil, err
	}

	// Group records to delete by name+type
	toDelete := make(map[string][]libdns.Record)
	for _, record := range records {
		rr := record.RR()
		key := rr.Name + ":" + rr.Type
		toDelete[key] = append(toDelete[key], record)
	}

	// For each name+type combination, find all existing values
	// and determine what action to take
	var deletedRecords []libdns.Record

	for key, deleteGroup := range toDelete {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name, recordType := parts[0], parts[1]

		var existingValues []libdns.Record
		for _, existing := range existingRecords {
			existingRR := existing.RR()
			if existingRR.Name == name && existingRR.Type == recordType {
				existingValues = append(existingValues, existing)
			}
		}

		if len(existingValues) == 0 {
			continue
		}

		deleteValues := make(map[string]bool)
		for _, rec := range deleteGroup {
			deleteValues[rec.RR().Data] = true
		}

		var remainingValues []libdns.Record
		for _, existing := range existingValues {
			if !deleteValues[existing.RR().Data] {
				remainingValues = append(remainingValues, existing)
			}
		}

		if len(remainingValues) == 0 {
			err := p.deleteRecordSet(ctx, zoneID, zone, name, recordType, existingValues)
			if err != nil {
				return nil, err
			}
		} else {
			_, err := p.setRecordSet(ctx, zoneID, zone, name, recordType, remainingValues)
			if err != nil {
				return nil, err
			}
		}

		for _, existing := range existingValues {
			if deleteValues[existing.RR().Data] {
				deletedRecords = append(deletedRecords, existing)
			}
		}
	}

	return deletedRecords, nil
}

// SetRecords sets the records in the zone, either by updating existing records
// or creating new ones. It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.init(ctx)

	zoneID, err := p.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	var updatedRecords []libdns.Record

	for _, record := range records {
		updatedRecord, err := p.updateRecord(ctx, zoneID, record, zone)
		if err != nil {
			return nil, err
		}
		updatedRecords = append(updatedRecords, updatedRecord)
	}

	return updatedRecords, nil
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
