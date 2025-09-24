# Provider-Specific Tests for Route53

This directory contains provider-specific tests for the Route53 libdns provider using the official [libdnstest package](https://github.com/libdns/libdns/tree/master/libdnstest). These tests verify the provider implementation against the real AWS Route53 API, ensuring all libdns interface methods work correctly with actual DNS operations.

## Prerequisites

1. **AWS Account**: You need an AWS account with Route53 access
2. **Hosted Zone**: Create a dedicated test hosted zone in Route53
3. **IAM Permissions**: Your AWS credentials need the following permissions:
   - `route53:ListResourceRecordSets`
   - `route53:GetChange`
   - `route53:ChangeResourceRecordSets`
   - `route53:ListHostedZonesByName`
   - `route53:ListHostedZones`

## How To Run

### Method 1: Using AWS Access Keys

1. **Set Environment Variables**:
```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"  # Optional, defaults to us-east-1
export ROUTE53_TEST_ZONE="test.example.com."  # Include trailing dot
```

### Method 2: Using AWS Profile

1. **Configure AWS Profile** (if not already done):
```bash
aws configure --profile myprofile
```

2. **Set Environment Variables**:
```bash
export AWS_PROFILE="myprofile"
export ROUTE53_TEST_ZONE="test.example.com."  # Include trailing dot
```

### Method 3: Using .env File

1. **Copy and Configure .env**:
```bash
cp .env.example .env
# Edit .env with your credentials
```

2. **Run Tests**:
```bash
set -a && source .env && set +a && go test -v
```

### Method 4: Using IAM Roles (EC2/ECS/Lambda)

If running on AWS infrastructure with IAM roles attached, just set:
```bash
export ROUTE53_TEST_ZONE="test.example.com."
go test -v
```

## What Gets Tested

- **Core Operations**: GetRecords, AppendRecords, SetRecords, DeleteRecords
- **Zone Listing**: ListZones (if provider implements it)
- **All Record Types**: A, AAAA, CNAME, TXT, MX, SRV, CAA, NS, SVCB, HTTPS

**Note**: These tests interact directly with the Route53 API and do not perform actual DNS queries. They verify that the provider correctly manages records through the AWS API.

## Important Warnings

> [!WARNING]
> **These tests create and delete real DNS records in your Route53 hosted zone!**
>
> - Use a **dedicated test zone** that doesn't host production DNS records
> - All test records are prefixed with "test-" but bugs could cause data loss
> - The tests attempt cleanup, but manual cleanup may be needed if tests fail
> - AWS Route53 operations incur charges (though minimal for testing)

## Cost Considerations

Route53 charges for:
- Hosted zones (~$0.50/month per zone)
- DNS queries (~$0.40 per million queries) - **Note: These tests do not perform DNS queries, only API operations**
- Record operations are free

For testing purposes, costs should be minimal since the tests only use the Route53 API to manage records, not actual DNS resolution.