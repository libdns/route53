package main

import (
	"context"
	"log"

	"github.com/jeremyforan/route53"
)

func main() {
	p := &route53.Provider{}
	rs, err := p.GetRecords(context.Background(), "example.com.")
	if err != nil {
		log.Fatalln(err)
	}
	for _, r := range rs {
		log.Printf("type: %v, value: %v, ttl: %v", r.RR().Type, r.RR().Data, r.RR().TTL)
	}
}
