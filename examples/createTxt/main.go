package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/libdns/libdns"
	"github.com/libdns/route53"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <zone>\nExample: %s example.com.", os.Args[0], os.Args[0])
	}
	zone := os.Args[1]

	// Provider will use AWS credentials from environment or AWS config
	p := &route53.Provider{}
	ctx := context.Background()

	// Create multiple TXT records with the same name
	// Route53 will combine these into a single ResourceRecordSet
	records := []libdns.Record{
		libdns.TXT{
			Name:  "example-txt",
			Text:  `This string includes "quotation marks".`,
			TTL:   300 * time.Second,
		},
		libdns.TXT{
			Name:  "example-txt",
			Text:  `The last character in this string is an accented e: é`,
			TTL:   300 * time.Second,
		},
		libdns.TXT{
			Name:  "example-txt",
			Text:  "v=spf1 ip4:192.168.0.1/16 -all",
			TTL:   300 * time.Second,
		},
	}

	// AppendRecords will add these records to the zone
	fmt.Printf("Creating TXT records in zone %s...\n", zone)
	created, err := p.AppendRecords(ctx, zone, records)
	if err != nil {
		log.Fatalf("Failed to create records: %v", err)
	}

	fmt.Printf("\nCreated %d TXT records:\n", len(created))
	for _, record := range created {
		rr := record.RR()
		fmt.Printf("TXT: {Name:%s TTL:%s Type:%s Data:%s}\n",
			rr.Name, rr.TTL, rr.Type, rr.Data)
	}

	fmt.Printf("\nCleaning up...\n")
	deleted, err := p.DeleteRecords(ctx, zone, created)
	if err != nil {
		log.Fatalf("Failed to delete records: %v", err)
	}
	fmt.Printf("Deleted %d records\n", len(deleted))
}
