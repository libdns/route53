package route53

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	r53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/libdns/libdns"
)

// NewSession initializes the AWS client
func (p *Provider) NewSession() error {
	if p.MaxRetries == 0 {
		p.MaxRetries = 5
	}

	config := &aws.Config{
		MaxRetries: aws.Int(p.MaxRetries),
	}
	if p.AccessKeyId != "" {
		config = config.WithCredentials(credentials.NewStaticCredentials(p.AccessKeyId, p.SecretAccessKey, ""))
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Profile:           p.AWSProfile,
		Config:            *config,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return err
	}

	p.client = r53.New(sess)

	return nil
}

func (p *Provider) getRecords(ctx context.Context, zoneID string, zone string) ([]libdns.Record, error) {
	getRecordsInput := &r53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		MaxItems:     aws.Int32(1000),
	}

	var records []libdns.Record
	var recordSets []types.ResourceRecordSet

	for {
		getRecordResult, err := p.client.ListResourceRecordSets(ctx, getRecordsInput)
		if err != nil {
			var nshze *types.NoSuchHostedZone
			var iie *types.InvalidInput
			if errors.As(err, &nshze) {
				return records, fmt.Errorf("NoSuchHostedZone: %s", err)
			} else if errors.As(err, &iie) {
				return records, fmt.Errorf("InvalidInput: %s", err)
			} else {
				return records, err
			}
		}

		recordSets = append(recordSets, getRecordResult.ResourceRecordSets...)
		if getRecordResult.IsTruncated {
			getRecordsInput.StartRecordName = getRecordResult.NextRecordName
			getRecordsInput.StartRecordType = getRecordResult.NextRecordType
			getRecordsInput.StartRecordIdentifier = getRecordResult.NextRecordIdentifier
		} else {
			break
		}
	}

	for _, rrset := range recordSets {
		for _, rrsetRecord := range rrset.ResourceRecords {
			record := libdns.Record{
				Name:  libdns.AbsoluteName(*rrset.Name, zone),
				Value: *rrsetRecord.Value,
				Type:  string(rrset.Type),
				TTL:   time.Duration(*rrset.TTL) * time.Second,
			}

			records = append(records, record)
		}
	}

	return records, nil
}

func (p *Provider) getZoneID(ctx context.Context, zoneName string) (string, error) {
	getZoneInput := &r53.ListHostedZonesByNameInput{
		DNSName:  aws.String(zoneName),
		MaxItems: aws.Int32(1),
	}

	getZoneResult, err := p.client.ListHostedZonesByName(ctx, getZoneInput)
	if err != nil {
		var idne *types.InvalidDomainName
		var iie *types.InvalidInput
		if errors.As(err, &idne) {
			return "", fmt.Errorf("InvalidDomainName: %s", err)
		} else if errors.As(err, &iie) {
			return "", fmt.Errorf("InvalidInput: %s", err)
		} else {
			return "", err
		}
	}

	if len(getZoneResult.HostedZones) > 0 {
		if *getZoneResult.HostedZones[0].Name == zoneName {
			return *getZoneResult.HostedZones[0].Id, nil
		}
	}

	return "", fmt.Errorf("HostedZoneNotFound: No zones found for the domain %s", zoneName)
}

func (p *Provider) createRecord(ctx context.Context, zoneID string, record libdns.Record, zone string) (libdns.Record, error) {
	// AWS Route53 TXT record value must be enclosed in quotation marks on create
	if record.Type == "TXT" {
		record.Value = strconv.Quote(record.Value)
	}

	createInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionCreate,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String(libdns.AbsoluteName(record.Name, zone)),
						ResourceRecords: []types.ResourceRecord{
							{
								Value: aws.String(record.Value),
							},
						},
						TTL:  aws.Int64(int64(record.TTL.Seconds())),
						Type: types.RRType(record.Type),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	err := p.applyChange(ctx, createInput)
	if err != nil {
		return record, err
	}

	return record, nil
}

func (p *Provider) updateRecord(ctx context.Context, zoneID string, record libdns.Record, zone string) (libdns.Record, error) {
	// AWS Route53 TXT record value must be enclosed in quotation marks on update
	if record.Type == "TXT" {
		record.Value = strconv.Quote(record.Value)
	}

	updateInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String(libdns.AbsoluteName(record.Name, zone)),
						ResourceRecords: []types.ResourceRecord{
							{
								Value: aws.String(record.Value),
							},
						},
						TTL:  aws.Int64(int64(record.TTL.Seconds())),
						Type: types.RRType(record.Type),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	err := p.applyChange(ctx, updateInput)
	if err != nil {
		return record, err
	}

	return record, nil
}

func (p *Provider) deleteRecord(ctx context.Context, zoneID string, record libdns.Record, zone string) (libdns.Record, error) {
	deleteInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionDelete,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String(libdns.AbsoluteName(record.Name, zone)),
						ResourceRecords: []types.ResourceRecord{
							{
								Value: aws.String(record.Value),
							},
						},
						TTL:  aws.Int64(int64(record.TTL.Seconds())),
						Type: types.RRType(record.Type),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	err := p.applyChange(ctx, deleteInput)
	if err != nil {
		return record, err
	}

	return record, nil
}

func (p *Provider) applyChange(ctx context.Context, input *r53.ChangeResourceRecordSetsInput) error {
	changeResult, err := p.client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		var nshze *types.NoSuchHostedZone
		var icbe *types.InvalidChangeBatch
		var iie *types.InvalidInput
		var prnce *types.PriorRequestNotComplete
		if errors.As(err, &nshze) {
			return fmt.Errorf("NoSuchHostedZone: %s", err)
		} else if errors.As(err, &icbe) {
			return fmt.Errorf("InvalidChangeBatch: %s", err)
		} else if errors.As(err, &iie) {
			return fmt.Errorf("InvalidInput: %s", err)
		} else if errors.As(err, &prnce) {
			return fmt.Errorf("PriorRequestNotComplete: %s", err)
		} else {
			return err
		}
	}

	changeInput := &r53.GetChangeInput{
		Id: changeResult.ChangeInfo.Id,
	}

	// Wait for the RecordSetChange status to be "INSYNC"
	waiter := r53.NewResourceRecordSetsChangedWaiter(p.client)
	err = waiter.Wait(ctx, changeInput, p.MaxWaitDur)
	if err != nil {
		return err
	}

	return nil
}
