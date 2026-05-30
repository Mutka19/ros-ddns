package dns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"ros-ddns/internal/util/environment"
	"strconv"
	"strings"
	"time"
)

type GoDaddyDNSProvider struct {
	client     *http.Client
	recordName string
	zoneName   string
	apiKey     string
	apiSecret  string
	ttl        int
}

// Update sends an API request to the GoDaddy zone to update the specified DNS record
func (g *GoDaddyDNSProvider) Update(ctx context.Context, ip string, at time.Time) error {
	// Define API requst payload
	body := `[{"data":"` + ip + `", "ttl": ` + strconv.Itoa(g.ttl) + `}]`

	// Build request to update record via GoDaddy API
	req, err := http.NewRequestWithContext(ctx, "PUT", "https://api.godaddy.com/v1/domains/"+g.zoneName+"/records/A/"+g.recordName, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	req.Header.Set("Accept", "application/json")

	// Add auth headers to request
	req.Header.Add("Authorization", "sso-key "+g.apiKey+":"+g.apiSecret)

	// Make API request and check for errors
	res, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer res.Body.Close()

	// Ensure OK status returned
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", res.Status)
	}

	return nil
}

// New initializes and returns a pointer to GoDaddyDNSProvider
func NewGoDaddyProvider() (Provider, error) {
	apiKey, err := environment.ReadSecret("dns_api_key")
	if err != nil || apiKey == "" {
		return nil, errors.New("dns_api_key secret is required")
	}

	apiSecret, err := environment.ReadSecret("dns_api_secret")
	if err != nil || apiSecret == "" {
		return nil, errors.New("dns_api_secret secret is required")
	}

	zoneName := os.Getenv("DNS_ZONE_NAME")
	if zoneName == "" {
		return nil, errors.New("DNS_ZONE_NAME environment variable is required")
	}

	recordName := os.Getenv("DNS_RECORD_NAME")
	if recordName == "" {
		return nil, errors.New("DNS_RECORD_NAME environment variable is required")
	}

	// Default TTL logic
	ttl := 60
	if ttlStr := os.Getenv("RECORD_TTL"); ttlStr != "" {
		parsedTTL, err := strconv.Atoi(ttlStr)
		if err != nil || parsedTTL < 600 {
			return nil, fmt.Errorf("invalid TTL value (%s): must be a valid integer no lower than 600", ttlStr)
		}
		ttl = parsedTTL
	}

	// Return a pointer to the newly created struct
	return &GoDaddyDNSProvider{
		client:     &http.Client{Timeout: 10 * time.Second},
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		zoneName:   zoneName,
		recordName: recordName,
		ttl:        ttl,
	}, nil
}

func init() {
	// Register GoDaddy as provider when package is initialized
	RegisterProvider("godaddy", NewGoDaddyProvider)
}
