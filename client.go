package route53

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	r53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/libdns/libdns"
)

func (p *Provider) setRecordSet(ctx context.Context, zoneID, zone, name, recordType string, records []libdns.Record) ([]libdns.Record, error) {
	var resourceRecords []types.ResourceRecord
	for _, record := range records {
		rr := record.RR()
		resourceRecords = append(resourceRecords, marshalRecord(rr)...)
	}

	// Use the TTL from the first record
	ttl := int64(300) // default
	if len(records) > 0 {
		ttl = int64(records[0].RR().TTL.Seconds())
	}

	// Use UPSERT to replace the entire record set
	input := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(ttl),
						Type:            types.RRType(recordType),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	err := p.applyChange(ctx, input)
	if err != nil {
		return nil, err
	}

	return records, nil
}

func (p *Provider) deleteRecordSet(ctx context.Context, zoneID, zone, name, recordType string, records []libdns.Record) error {
	var resourceRecords []types.ResourceRecord
	for _, record := range records {
		rr := record.RR()
		resourceRecords = append(resourceRecords, marshalRecord(rr)...)
	}

	// Use the TTL from the first record
	ttl := int64(300) // default
	if len(records) > 0 {
		ttl = int64(records[0].RR().TTL.Seconds())
	}

	// Use DELETE action to remove the entire record set
	input := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionDelete,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(ttl),
						Type:            types.RRType(recordType),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	return p.applyChange(ctx, input)
}

func (p *Provider) init(ctx context.Context) {
	if p.client != nil {
		return
	}

	if p.MaxRetries == 0 {
		p.MaxRetries = 5
	}

	p.MaxWaitDur *= time.Second
	if p.MaxWaitDur == 0 {
		p.MaxWaitDur = time.Minute
	}

	opts := make([]func(*config.LoadOptions) error, 0)
	opts = append(opts,
		config.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), p.MaxRetries)
		}),
	)

	profile := p.Profile
	if profile == "" {
		profile = p.AWSProfile
	}

	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	if p.Region != "" {
		opts = append(opts, config.WithRegion(p.Region))
	}

	if p.AccessKeyId != "" && p.SecretAccessKey != "" {
		token := p.SessionToken
		if token == "" {
			token = p.Token
		}

		opts = append(opts,
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(p.AccessKeyId, p.SecretAccessKey, token)),
		)
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		log.Fatalf("route53: unable to load AWS SDK config, %v", err)
	}

	p.client = r53.NewFromConfig(cfg)
}

func chunkString(s string, chunkSize int) []string {
	var chunks []string
	for i := 0; i < len(s); i += chunkSize {
		end := i + chunkSize
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

func parseRecordSet(set types.ResourceRecordSet, zone string) ([]libdns.Record, error) {
	records := make([]libdns.Record, 0)

	// Route53 returns TXT & SPF records with quotes around them.
	// https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/ResourceRecordTypes.html#TXTFormat
	var ttl int64
	if set.TTL != nil {
		ttl = *set.TTL
	}

	rtype := string(set.Type)
	relativeName := libdns.RelativeName(*set.Name, zone)

	for _, record := range set.ResourceRecords {
		value := *record.Value
		switch rtype {
		case "TXT", "SPF":
			rows := strings.Split(value, "\n")
			for _, row := range rows {
				parts := strings.Split(row, `" "`)
				if len(parts) > 0 {
					parts[0] = strings.TrimPrefix(parts[0], `"`)
					parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], `"`)
				}

				row = strings.Join(parts, "")
				row = unquote(row)

				rr := libdns.RR{
					Name: relativeName,
					Data: row,
					Type: rtype,
					TTL:  time.Duration(ttl) * time.Second,
				}
				parsedRecord, err := rr.Parse()
				if err != nil {
					return nil, fmt.Errorf("failed to parse %s record %s: %w", rtype, relativeName, err)
				}
				records = append(records, parsedRecord)
			}
		default:
			rr := libdns.RR{
				Name: relativeName,
				Data: value,
				Type: rtype,
				TTL:  time.Duration(ttl) * time.Second,
			}
			parsedRecord, err := rr.Parse()
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s record %s: %w", rtype, relativeName, err)
			}
			records = append(records, parsedRecord)
		}
	}

	return records, nil
}

func marshalRecord(record libdns.RR) []types.ResourceRecord {
	resourceRecords := make([]types.ResourceRecord, 0)

	// Route53 requires TXT & SPF records to be quoted.
	// https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/ResourceRecordTypes.html#TXTFormat
	switch record.Type {
	case "TXT", "SPF":
		strs := make([]string, 0)
		if len(record.Data) > 255 {
			strs = append(strs, chunkString(record.Data, 255)...)
		} else {
			strs = append(strs, record.Data)
		}

		// Quote strings
		for i, str := range strs {
			strs[i] = quote(str)
		}

		// Finally join chunks with spaces
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(strings.Join(strs, " ")),
		})
	default:
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(record.Data),
		})
	}

	return resourceRecords
}

func (p *Provider) getRecords(ctx context.Context, zoneID string, zone string) ([]libdns.Record, error) {
	getRecordsInput := &r53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		MaxItems:     aws.Int32(1000),
	}

	var records []libdns.Record

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

		for _, s := range getRecordResult.ResourceRecordSets {
			parsedRecords, err := parseRecordSet(s, zone)
			if err != nil {
				return records, fmt.Errorf("failed to parse record set: %w", err)
			}
			records = append(records, parsedRecords...)
		}

		if getRecordResult.IsTruncated {
			getRecordsInput.StartRecordName = getRecordResult.NextRecordName
			getRecordsInput.StartRecordType = getRecordResult.NextRecordType
			getRecordsInput.StartRecordIdentifier = getRecordResult.NextRecordIdentifier
		} else {
			break
		}
	}

	return records, nil
}

func (p *Provider) getZoneID(ctx context.Context, zoneName string) (string, error) {
	if p.HostedZoneID != "" {
		return "/hostedzone/" + p.HostedZoneID, nil
	}

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

	matchingZones := []types.HostedZone{}

	if len(getZoneResult.HostedZones) > 0 {
		for z := 0; z < len(getZoneResult.HostedZones); z++ {
			if *getZoneResult.HostedZones[z].Name == zoneName {
				matchingZones = append(matchingZones, getZoneResult.HostedZones[z])
			}
		}
	}

	if len(matchingZones) == 1 {
		return *matchingZones[0].Id, nil
	}

	// If multiple zones matched the name
	if len(matchingZones) > 1 {
		// Select the first public (i.e. ot-private) zone as a best guess.
		for m := 0; m < len(matchingZones); m++ {
			if !matchingZones[m].Config.PrivateZone {
				return *matchingZones[m].Id, nil
			}
		}
		// All zone were private, give up and return.
		// Historically we always returned the first match without checking for public/private
		return *matchingZones[0].Id, nil
	}

	return "", fmt.Errorf("HostedZoneNotFound: No zones found for the domain %s", zoneName)
}

func (p *Provider) createRecord(ctx context.Context, zoneID string, record libdns.Record, zone string) (libdns.Record, error) {
	resourceRecords := marshalRecord(record.RR())
	createInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionCreate,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(record.RR().Name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(int64(record.RR().TTL.Seconds())),
						Type:            types.RRType(record.RR().Type),
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
	// Route53's UPSERT replaces the entire ResourceRecordSet
	// For TXT records with the same name, we might want to preserve other values
	// But for libdns SetRecords, we should replace everything
	resourceRecords := marshalRecord(record.RR())
	updateInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(record.RR().Name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(int64(record.RR().TTL.Seconds())),
						Type:            types.RRType(record.RR().Type),
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
	if record.RR().Type == "TXT" {
		txtRecords, err := p.getTxtRecordsFor(ctx, zoneID, zone, record.RR().Name)
		if err != nil {
			return record, err
		}

		switch {
		// If there is only one record, we can delete the entire record set.
		case len(txtRecords) == 1:
			resourceRecords = append(resourceRecords, marshalRecord(record.RR())...)
		// If there are multiple records, we need to upsert the remaining records.
		case len(txtRecords) > 1:
			action = types.ChangeActionUpsert
			resourceRecords = make([]types.ResourceRecord, 0)
			for _, r := range txtRecords {
				if record.RR().Data != r.Data {
					resourceRecords = append(resourceRecords, marshalRecord(r)...)
				}
			}
		}
	} else {
		// For non-TXT records, we need to include the record data
		resourceRecords = append(resourceRecords, marshalRecord(record.RR())...)
	}

	deleteInput := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: action,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(libdns.AbsoluteName(record.RR().Name, zone)),
						ResourceRecords: resourceRecords,
						TTL:             aws.Int64(int64(record.RR().TTL.Seconds())),
						Type:            types.RRType(record.RR().Type),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}

	err := p.applyChange(ctx, deleteInput)
	if err != nil {
		var nfe *types.InvalidChangeBatch
		if record.RR().Type == "TXT" && errors.As(err, &nfe) {
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

func (p *Provider) getTxtRecords(ctx context.Context, zoneID string, zone string) ([]libdns.RR, error) {
	txtRecords := make([]libdns.RR, 0)
	records, err := p.getRecords(ctx, zoneID, zone)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.RR().Type == "TXT" {
			txtRecords = append(txtRecords, r.RR())
		}
	}
	return txtRecords, nil
}

func (p *Provider) getTxtRecordsFor(ctx context.Context, zoneID string, zone string, name string) ([]libdns.RR, error) {
	txtRecords, err := p.getTxtRecords(ctx, zoneID, zone)
	if err != nil {
		return nil, err
	}
	records := make([]libdns.RR, 0)
	for _, r := range txtRecords {
		if libdns.AbsoluteName(name, zone) == r.Name {
			records = append(records, r)
		}
	}
	return records, nil
}
