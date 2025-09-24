package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/libdns/route53"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <zone>\nExample: %s example.com.", os.Args[0], os.Args[0])
	}
	zone := os.Args[1]

	// Provider will use AWS credentials from environment or AWS config
	p := &route53.Provider{}

	// Get all records from the zone
	records, err := p.GetRecords(context.Background(), zone)
	if err != nil {
		log.Fatalf("Failed to get records: %v", err)
	}

	fmt.Printf("Records in zone %s:\n\n", zone)

	for _, record := range records {
		typeName := fmt.Sprintf("%T", record)
		if idx := strings.LastIndex(typeName, "."); idx != -1 {
			typeName = typeName[idx+1:]
		}

		rr := record.RR()
		fmt.Printf("%s: {Name:%s TTL:%s Type:%s Data:%s}\n",
			typeName, rr.Name, rr.TTL, rr.Type, rr.Data)
	}

	fmt.Printf("\nTotal: %d records\n", len(records))
}
