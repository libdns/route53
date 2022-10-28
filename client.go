package route53

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
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

// init initializes the AWS client
func (p *Provider) init(ctx context.Context) {
	if p.client != nil {
		return
	}

	if p.MaxRetries == 0 {
		p.MaxRetries = 5
	}
	if p.Region == "" {
		p.Region = "us-east-1"
	}
	if p.MaxWaitDur == 0 {
		p.MaxWaitDur = time.Minute
	}

	opts := make([]func(*config.LoadOptions) error, 0)
	opts = append(opts,
		config.WithSharedConfigProfile(p.AWSProfile),
		config.WithRegion(p.Region),
		config.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), p.MaxRetries)
		}),
	)
	if p.AccessKeyId != "" && p.SecretAccessKey != "" {
		opts = append(opts,
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(p.AccessKeyId, p.SecretAccessKey, p.Token)),
		)
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)

	if err != nil {
		log.Fatal(err)
	}

	p.client = r53.NewFromConfig(cfg)
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
	var re = regexp.MustCompile(`(?m)" "`)

	for _, rrset := range recordSets {
		for _, rrsetRecord := range rrset.ResourceRecords {
			rtype := rrset.Type
			value := *rrsetRecord.Value
			// Route53 returns TXT & SPF records with quotes around them.
			// https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/ResourceRecordTypes.html#TXTFormat
			switch rtype {
			case types.RRTypeTxt, types.RRTypeSpf:
				var err error
				value = re.ReplaceAllString(value, "")
				//fmt.Println(value)
				value, err = strconv.Unquote(value)
				if err != nil {
					return records, fmt.Errorf("Error unquoting TXT/SPF record: %s", err)
				}
			}
			record := libdns.Record{
				Name:  *rrset.Name,
				Value: value,
				Type:  string(rtype),
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
	switch record.Type {
	case "TXT":
		return p.updateRecord(ctx, zoneID, record, zone)
	case "SPF":
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
	resourceRecords := make([]types.ResourceRecord, 0)
	// AWS Route53 TXT record value must be enclosed in quotation marks on update
	switch record.Type {
	case "SPF", "TXT":
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(strconv.Quote(record.Value)),
		})
	}
	if record.Type == "TXT" {
		txtRecords, err := p.getTxtRecordsFor(ctx, zoneID, zone, record.Name)
		if err != nil {
			return record, err
		}
		for _, r := range txtRecords {
			if record.Value != r.Value {
				resourceRecords = append(resourceRecords, types.ResourceRecord{
					Value: aws.String(strconv.Quote(r.Value)),
				})
			}
		}
	}

	updateInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(record.Name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(int64(record.TTL.Seconds())),
						Type:            types.RRType(record.Type),
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
	action := types.ChangeActionDelete
	resourceRecords := make([]types.ResourceRecord, 0)
	// AWS Route53 TXT record value must be enclosed in quotation marks on update
	switch record.Type {
	case "SPF", "TXT":
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(strconv.Quote(record.Value)),
		})
	}
	if record.Type == "TXT" {
		txtRecords, err := p.getTxtRecordsFor(ctx, zoneID, zone, record.Name)
		if err != nil {
			return record, err
		}
		switch {
		case len(txtRecords) > 0 && txtRecords[0].Value != record.Value,
			len(txtRecords) > 1:
			action = types.ChangeActionUpsert
			resourceRecords = make([]types.ResourceRecord, 0)
		}
		for _, r := range txtRecords {
			if record.Value != r.Value {
				resourceRecords = append(resourceRecords, types.ResourceRecord{
					Value: aws.String(strconv.Quote(r.Value)),
				})
			}
		}
	}

	deleteInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: action,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(record.Name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(int64(record.TTL.Seconds())),
						Type:            types.RRType(record.Type),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	err := p.applyChange(ctx, deleteInput)
	if err != nil {
		var nfe *types.InvalidChangeBatch
		if record.Type == "TXT" && errors.As(err, &nfe) {
			return record, nil
		}
		return record, err
	}

	return record, nil
}

func (p *Provider) applyChange(ctx context.Context, input *r53.ChangeResourceRecordSetsInput) error {
	changeResult, err := p.client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return err
	}

	// Waiting for propagation if it's set in the provider config.
	if p.WaitForPropagation {
		changeInput := &r53.GetChangeInput{
			Id: changeResult.ChangeInfo.Id,
		}

		// Wait for the RecordSetChange status to be "INSYNC"
		waiter := r53.NewResourceRecordSetsChangedWaiter(p.client)
		err = waiter.Wait(ctx, changeInput, p.MaxWaitDur)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Provider) getTxtRecords(ctx context.Context, zoneID string, zone string) ([]libdns.Record, error) {
	txtRecords := make([]libdns.Record, 0)
	records, err := p.getRecords(ctx, zoneID, zone)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.Type == "TXT" {
			txtRecords = append(txtRecords, r)
		}
	}
	return txtRecords, nil
}

func (p *Provider) getTxtRecordsFor(ctx context.Context, zoneID string, zone string, name string) ([]libdns.Record, error) {
	txtRecords, err := p.getTxtRecords(ctx, zoneID, zone)
	if err != nil {
		return nil, err
	}
	records := make([]libdns.Record, 0)
	for _, r := range txtRecords {
		if libdns.AbsoluteName(name, zone) == r.Name {
			records = append(records, r)
		}
	}
	return records, nil
}
