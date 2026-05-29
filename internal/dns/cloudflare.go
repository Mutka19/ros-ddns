package dns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"ros-ddns/internal/util"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
)

type CloudflareDNSProvider struct {
	client     *cloudflare.Client
	recordId   string
	recordName string
	zoneId     string
	ttl        int
}

// New initializes and returns a pointer to CloudflareDNSProvider
func New() (*CloudflareDNSProvider, error) {
	apiToken, err := util.ReadSecret("dns_api_token")
	if err != nil || apiToken == "" {
		return nil, errors.New("dns_api_token secret is required")
	}

	zoneId := os.Getenv("DNS_ZONE_ID")
	if zoneId == "" {
		return nil, errors.New("DNS_ZONE_ID environment variable is required")
	}

	recordId := os.Getenv("DNS_RECORD_ID")
	if recordId == "" {
		return nil, errors.New("DNS_RECORD_ID environment variable is required")
	}

	recordName := os.Getenv("DNS_RECORD_NAME")
	if recordName == "" {
		return nil, errors.New("DNS_RECORD_NAME environment variable is required")
	}

	// Default TTL logic
	ttl := 60
	if ttlStr := os.Getenv("RECORD_TTL"); ttlStr != "" {
		parsedTTL, err := strconv.Atoi(ttlStr)
		if err != nil || parsedTTL < 30 {
			return nil, fmt.Errorf("invalid TTL value (%s): must be a valid integer no lower than 30", ttlStr)
		}
		ttl = parsedTTL
	}

	// Return a pointer to the newly created struct
	return &CloudflareDNSProvider{
		client:     cloudflare.NewClient(option.WithAPIToken(apiToken)),
		recordId:   recordId,
		recordName: recordName,
		zoneId:     zoneId,
		ttl:        ttl,
	}, nil
}

// Update sends an API request to the Cloudflare zone to update the specified DNS record
func (c *CloudflareDNSProvider) Update(ctx context.Context, ip string, at time.Time) error {
	_, err := c.client.DNS.Records.Edit(
		ctx,
		c.recordId,
		dns.RecordEditParams{
			ZoneID: cloudflare.F(c.zoneId),
			Body: dns.ARecordParam{
				Content: cloudflare.F(ip),
				Name:    cloudflare.F(c.recordName),
				TTL:     cloudflare.F(dns.TTL(c.ttl)),
				Type:    cloudflare.F(dns.ARecordType("A")),
				Comment: cloudflare.F("Last update via ROS ddnsd: " + at.UTC().Format(time.UnixDate)),
			},
		},
	)
	return err
}
