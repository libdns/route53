package main

import (
	"context"
	"log"

	"github.com/libdns/libdns"
	"github.com/libdns/route53"
)

func main() {
	p := &route53.Provider{}
	ctx := context.Background()
	_, err := p.AppendRecords(ctx, "charm.sh.", []libdns.Record{
		{
			Name:  "abctxt",
			Value: `This string includes "quotation marks".`,
			Type:  "TXT",
		},
		{
			Name:  "abctxt",
			Value: `The last character in this string is an accented e specified in octal format: \351`,
			Type:  "TXT",
		},
		{
			Name:  "abctxt",
			Value: "v=spf1 ip4:192.168.0.1/16 -all",
			Type:  "TXT",
		},
	})
	if err != nil {
		log.Fatalln(err)
	}
}
