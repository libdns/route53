Route53 for `libdns`
=======================

[![godoc reference](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/libdns/route53)

This package implements the [libdns interfaces](https://github.com/libdns/libdns) for AWS [Route53](https://aws.amazon.com/route53/).

## Example

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libdns/route53"
)

func main() {
	// greate a new Route53 provider instance
	provider := &route53.Provider{
		AccessKeyId:     "YOUR_ACCESS_KEY_ID",
		SecretAccessKey: "YOUR_SECRET_ACCESS_KEY",
		Region:          "us-east-1",
	}

	ctx := context.Background()
	zone := "example.com."

	// get all records for the zone
	records, err := provider.GetRecords(ctx, zone)
	if err != nil {
		panic(err)
	}

	for _, record := range records {
		fmt.Printf("%s %s %s %d\n", record.Name, record.Type, record.Value, record.TTL/time.Second)
	}
}
```

## Authenticating

This package supports all the credential configuration methods described in the [AWS Developer Guide](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/#specifying-credentials), such as `Environment Variables`, `Shared configuration files`, the `AWS Credentials file` located in `.aws/credentials`, and `Static Credentials`. You may also pass in static credentials directly (or via caddy's configuration).

The following IAM policy is a minimal working example to give `libdns` permissions to manage DNS records:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "",
            "Effect": "Allow",
            "Action": [
                "route53:ListResourceRecordSets",
                "route53:GetChange",
                "route53:ChangeResourceRecordSets"
            ],
            "Resource": [
                "arn:aws:route53:::hostedzone/ZABCD1EFGHIL",
                "arn:aws:route53:::change/*"
            ]
        },
        {
            "Sid": "",
            "Effect": "Allow",
            "Action": [
                "route53:ListHostedZonesByName",
                "route53:ListHostedZones"
            ],
            "Resource": "*"
        }
    ]
}
```

### Running in Docker on EC2 with Instance Roles

When running this provider in a Docker container on EC2 instances that use IAM instance roles, you need to ensure that the container can access the EC2 metadata service. By default, IMDSv2 (Instance Metadata Service Version 2) limits the hop count to 1, which prevents Docker containers from accessing the metadata service.

Instances created through the AWS Console typically have a hop limit of 2 by default and won't have this issue. This configuration is usually needed for instances created programmatically or with older configurations.

To enable Docker containers to use EC2 instance roles, configure the instance metadata options with an increased hop limit:

```bash
aws ec2 modify-instance-metadata-options \
    --instance-id <instance-id> \
    --http-put-response-hop-limit 2 \
    --http-endpoint enabled
```

Or when launching an instance:

```bash
aws ec2 run-instances \
    --metadata-options "HttpEndpoint=enabled,HttpPutResponseHopLimit=2" \
    # ... other parameters
```

For more information, see the [AWS EC2 Instance Metadata Options documentation](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_InstanceMetadataOptionsRequest.html).

## Contributing

Contributions are welcome! Please ensure that:

1. All code passes `golangci-lint` checks. Run the following before committing:
   ```bash
   golangci-lint run ./...
   ```

2. All tests pass:
   ```bash
   go test ./...
   ```

3. For integration tests, set up the required environment variables:
   ```bash
   export AWS_ACCESS_KEY_ID="your-key"
   export AWS_SECRET_ACCESS_KEY="your-secret"
   export ROUTE53_TEST_ZONE="test.example.com."
   cd libdnstest && go test -v
   ```

Please fix any linter issues before submitting a pull request. The project maintains strict code quality standards to ensure maintainability.
