package route53

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/libdns/libdns"
)

func TestTXTMarshalling(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string with quotes",
			input:    `This string includes "quotation marks".`,
			expected: `"This string includes \"quotation marks\"."`,
		},
		{
			name:     "string with backslashes",
			input:    `This string includes \backslashes\`,
			expected: `"This string includes \\backslashes\\"`,
		},
		{
			name:     "string with special characters",
			input:    `The last character in this string is an accented e specified in octal format: é`,
			expected: `"The last character in this string is an accented e specified in octal format: \351"`,
		},
		{
			name:     "simple",
			input:    "v=spf1 ip4:192.168.0.1/16 -all",
			expected: `"v=spf1 ip4:192.168.0.1/16 -all"`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := quote(c.input)
			if actual != c.expected {
				t.Errorf("expected %s, got %s", c.expected, actual)
			}
		})
	}
}

func TestTXTUnmarhalling(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string with quotes",
			input:    `"This string includes \"quotation marks\"."`,
			expected: `This string includes "quotation marks".`,
		},
		{
			name:     "string with backslashes",
			input:    `"This string includes \\backslashes\\"`,
			expected: `This string includes \backslashes\`,
		},
		{
			name:     "string with special characters",
			input:    `"The last character in this string is an accented e specified in octal format: \351"`,
			expected: `The last character in this string is an accented e specified in octal format: é`,
		},
		{
			name:     "simple",
			input:    `"v=spf1 ip4:192.168.0.1/16 -all"`,
			expected: "v=spf1 ip4:192.168.0.1/16 -all",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := unquote(c.input)
			if actual != c.expected {
				t.Errorf("expected %s, got %s", c.expected, actual)
			}
		})
	}
}

func TestParseRecordSet(t *testing.T) {
	cases := []struct {
		name     string
		input    types.ResourceRecordSet
		expected []libdns.Record
	}{
		{
			name: "A record",
			input: types.ResourceRecordSet{
				Name: aws.String(""),
				Type: types.RRTypeA,
				ResourceRecords: []types.ResourceRecord{
					{
						Value: aws.String("127.0.0.1"),
					},
				},
			},
			expected: []libdns.Record{
				{
					Type:  "A",
					Name:  "",
					Value: "127.0.0.1",
				},
			},
		},
		{
			name: "CNAME record",
			input: types.ResourceRecordSet{
				Name: aws.String("*"),
				Type: types.RRTypeCname,
				ResourceRecords: []types.ResourceRecord{
					{
						Value: aws.String("example.com"),
					},
				},
			},
			expected: []libdns.Record{
				{
					Type:  "CNAME",
					Name:  "*",
					Value: "example.com",
				},
			},
		},
		{
			name: "TXT record",
			input: types.ResourceRecordSet{
				Name: aws.String("test"),
				Type: types.RRTypeTxt,
				ResourceRecords: []types.ResourceRecord{
					{
						Value: aws.String(`"This string includes \"quotation marks\"."`),
					},
					{
						Value: aws.String(`"This string includes \\backslashes\\"`),
					},
					{
						Value: aws.String(`"The last character in this string is an accented e specified in octal format: \351"`),
					},
					{
						Value: aws.String(`"String 1" "String 2" "String 3"`),
					},
				},
			},
			expected: []libdns.Record{
				{
					Type:  "TXT",
					Name:  "test",
					Value: `This string includes "quotation marks".`,
				},
				{
					Type:  "TXT",
					Name:  "test",
					Value: `This string includes \backslashes\`,
				},
				{
					Type:  "TXT",
					Name:  "test",
					Value: `The last character in this string is an accented e specified in octal format: é`,
				},
				{
					Type:  "TXT",
					Name:  "test",
					Value: `String 1String 2String 3`,
				},
			},
		},
		{
			name: "TXT long record",
			input: types.ResourceRecordSet{
				Name: aws.String("_testlong"),
				Type: types.RRTypeTxt,
				ResourceRecords: []types.ResourceRecord{
					{
						Value: aws.String(`"3gImdrsMGi6MzHi2rMviVqvwJbv7tXDPk6JvUEI2Fnl7sRF1bUSjNIe4qnatzomDu368bV6Q45qItkF wwnYoGBXNu1uclGvlPIIcGQd6wqBPzTtv0P83brCXJ59RJNLnAif8a3EQuLy88GmblPq 42uJpHTeNYnDRLQt8WvhRCYySX6bx" "vJtK8TZJtVRFbCgUrziRgQVzLwV4fn2hitpnItt U3Ke9IE5 gcs1Obx9kG8wkQ9h4qIxKDLVsmYdhuw4kdLmM2Qm6jJ3ZlSIaQWFP2eNLq5NwZfgATZiGRhr"`),
					},
				},
			},
			expected: []libdns.Record{
				{
					Type:  "TXT",
					Name:  "_testlong",
					Value: "3gImdrsMGi6MzHi2rMviVqvwJbv7tXDPk6JvUEI2Fnl7sRF1bUSjNIe4qnatzomDu368bV6Q45qItkF wwnYoGBXNu1uclGvlPIIcGQd6wqBPzTtv0P83brCXJ59RJNLnAif8a3EQuLy88GmblPq 42uJpHTeNYnDRLQt8WvhRCYySX6bxvJtK8TZJtVRFbCgUrziRgQVzLwV4fn2hitpnItt U3Ke9IE5 gcs1Obx9kG8wkQ9h4qIxKDLVsmYdhuw4kdLmM2Qm6jJ3ZlSIaQWFP2eNLq5NwZfgATZiGRhr",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := parseRecordSet(c.input)
			if len(actual) != len(c.expected) {
				t.Errorf("expected %d records, got %d", len(c.expected), len(actual))
			}
			for i, record := range actual {
				if record.Type != c.expected[i].Type {
					t.Errorf("expected type %s, got %s", c.expected[i].Type, record.Type)
				}
				if record.Name != c.expected[i].Name {
					t.Errorf("expected name %s, got %s", c.expected[i].Name, record.Name)
				}
				if record.Value != c.expected[i].Value {
					t.Errorf("expected value %s, got %s", c.expected[i].Value, record.Value)
				}
			}
		})
	}
}

func TestMarshalRecord(t *testing.T) {
	cases := []struct {
		name     string
		input    libdns.Record
		expected []types.ResourceRecord
	}{
		{
			name: "A record",
			input: libdns.Record{
				Type:  "A",
				Name:  "",
				Value: "127.0.0.1",
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String("127.0.0.1"),
				},
			},
		},
		{
			name: "A record with name",
			input: libdns.Record{
				Type:  "A",
				Name:  "test",
				Value: "127.0.0.1",
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String("127.0.0.1"),
				},
			},
		},
		{
			name: "TXT record",
			input: libdns.Record{
				Type:  "TXT",
				Name:  "",
				Value: "test",
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String(`"test"`),
				},
			},
		},
		{
			name: "TXT record with name",
			input: libdns.Record{
				Type:  "TXT",
				Name:  "test",
				Value: "test",
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String(`"test"`),
				},
			},
		},
		{
			name: "TXT record with long value",
			input: libdns.Record{
				Type:  "TXT",
				Name:  "test",
				Value: `3gImdrsMGi6MzHi2rMviVqvwJbv7tXDPk6JvUEI2Fnl7sRF1bUSjNIe4qnatzomDu368bV6Q45qItkF wwnYoGBXNu1uclGvlPIIcGQd6wqBPzTtv0P83brCXJ59RJNLnAif8a3EQuLy88GmblPq 42uJpHTeNYnDRLQt8WvhRCYySX6bxvJtK8TZJtVRFbCgUrziRgQVzLwV4fn2hitpnItt U3Ke9IE5 gcs1Obx9kG8wkQ9h4qIxKDLVsmYdhuw4kdLmM2Qm6jJ3ZlSIaQWFP2eNLq5NwZfgATZiGRhr`,
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String(`"3gImdrsMGi6MzHi2rMviVqvwJbv7tXDPk6JvUEI2Fnl7sRF1bUSjNIe4qnatzomDu368bV6Q45qItkF wwnYoGBXNu1uclGvlPIIcGQd6wqBPzTtv0P83brCXJ59RJNLnAif8a3EQuLy88GmblPq 42uJpHTeNYnDRLQt8WvhRCYySX6bxvJtK8TZJtVRFbCgUrziRgQVzLwV4fn2hitpnItt U3Ke9IE5 gcs1Obx9kG8wkQ9h4qIxKDLVsmYd" "huw4kdLmM2Qm6jJ3ZlSIaQWFP2eNLq5NwZfgATZiGRhr"`),
				},
			},
		},
		{
			name: "TXT record with a special character",
			input: libdns.Record{
				Type:  "TXT",
				Name:  "test",
				Value: `test é`,
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String(`"test \351"`),
				},
			},
		},
		{
			name: "TXT record with quotes",
			input: libdns.Record{
				Type:  "TXT",
				Name:  "test",
				Value: `"test"`,
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String(`"\"test\""`),
				},
			},
		},
		{
			name: "TXT record with backslashes",
			input: libdns.Record{
				Type:  "TXT",
				Name:  "test",
				Value: `\test\`,
			},
			expected: []types.ResourceRecord{
				{
					Value: aws.String(`"\\test\\"`),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := marshalRecord(c.input)
			if len(actual) != len(c.expected) {
				t.Errorf("expected %d records, got %d", len(c.expected), len(actual))
			}
			for i, record := range actual {
				if *record.Value != *c.expected[i].Value {
					t.Errorf("expected value %s, got %s", *c.expected[i].Value, *record.Value)
				}
			}
		})
	}
}

func TestMaxWaitDur(t *testing.T) {
	cases := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{
			name:     "default",
			input:    0,
			expected: 60 * time.Second,
		},
		{
			name:     "custom",
			input:    120,
			expected: 120 * time.Second,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			provider := Provider{MaxWaitDur: c.input}
			provider.init(context.TODO())
			actual := provider.MaxWaitDur
			if actual != c.expected {
				t.Errorf("expected %d, got %d", c.expected, actual)
			}
		})
	}
}
