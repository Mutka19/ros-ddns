package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"ros-ddns/internal/config"
)

type addrResponse struct {
	Address string `json:"address"`
}

func FetchIP(ctx context.Context, cfg config.Config, client *http.Client) (net.IP, error) {
	// Build request to fetch IP assigned to ROS DHCP client
	req, err := http.NewRequestWithContext(ctx, "GET", "https://"+cfg.ROSHost+"/rest/ip/dhcp-client/*1?.proplist=address", nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	// Set basic auth (ROS only allows this auth method)
	req.SetBasicAuth(cfg.ROSUser, cfg.ROSPass)

	// Make GET request
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", res.Status)
	}

	// Pull IP address from HTTP response body
	var result addrResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	ip, _, err := net.ParseCIDR(result.Address)
	if err != nil {
		return nil, fmt.Errorf("parsing CIDR %q: %w", result.Address, err)
	}

	return ip, nil
}
