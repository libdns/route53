module github.com/libdns/route53/examples

go 1.19

replace github.com/libdns/route53 => ../

require (
	github.com/libdns/libdns v0.2.1
	github.com/libdns/route53 v1.2.2
)

require (
	github.com/aws/aws-sdk-go-v2 v1.10.0 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.9.0 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.2.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53 v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.8.0 // indirect
	github.com/aws/smithy-go v1.8.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
)
