package main

import (
	"context"
	"log"

	"github.com/libdns/route53"
)

func main() {
	p := &route53.Provider{}
	rs, err := p.GetRecords(context.Background(), "example.com.")
	if err != nil {
		log.Fatalln(err)
	}
	for _, r := range rs {
		log.Printf("type: %v, value: %v, ttl: %v", r.Type, r.Value, r.TTL)
	}
}
