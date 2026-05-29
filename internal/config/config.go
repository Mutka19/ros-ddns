package config

import (
	"errors"
	"os"
	"ros-ddns/internal/util/environment"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the daemon.
type Config struct {
	HealthAddr      string
	WorkInterval    time.Duration
	ShutdownTimeout time.Duration
	ROSHost         string
	ROSUser         string
	ROSPass         string
	DNSAPIToken     string
	DNSZoneID       string
	DNSRecordID     string
	DNSRecordName   string
	RecordTTL       int
}

func SetConfig() (Config, error) {
	// Ensure all required variables and secrets are loaded
	rosHost := os.Getenv("ROS_HOST")
	if rosHost == "" {
		return Config{}, errors.New("ROS_HOST is required")
	}

	rosUser, err := environment.ReadSecret("ros_user")
	if err != nil || rosUser == "" {
		return Config{}, errors.New("ROS_USER secret is required")
	}

	rosPass, err := environment.ReadSecret("ros_pass")
	if err != nil || rosPass == "" {
		return Config{}, errors.New("ROS_PASS secret is required")
	}

	dnsApiToken, err := environment.ReadSecret("dns_api_token")
	if err != nil || dnsApiToken == "" {
		return Config{}, errors.New("DNS_API_TOKEN secret is required")
	}

	dnsZoneId := os.Getenv("DNS_ZONE_ID")
	if dnsZoneId == "" {
		return Config{}, errors.New("DNS_ZONE_ID is required")
	}

	dnsRecordId := os.Getenv("DNS_RECORD_ID")
	if dnsRecordId == "" {
		return Config{}, errors.New("DNS_RECORD_ID is required")
	}

	dnsRecordName := os.Getenv("DNS_RECORD_NAME")
	if dnsRecordName == "" {
		return Config{}, errors.New("DNS_RECORD_NAME is required")
	}

	// Check if optional variables are set, if not set defaults
	var recordTTL int
	ttlStr := os.Getenv("RECORD_TTL")
	if ttlStr == "" {
		recordTTL = 60
	} else {
		var err error
		recordTTL, err = strconv.Atoi(ttlStr)
		if err != nil || recordTTL < 30 {
			return Config{}, errors.New("Invalid TTL value (" + ttlStr + "), must be valid INT and no lower than 30")
		}
	}

	var interval int
	intervalStr := os.Getenv("CHECK_INTERVAL")
	if intervalStr == "" {
		interval = 30
	} else {
		var err error
		interval, err = strconv.Atoi(intervalStr)
		if err != nil || interval < 1 {
			return Config{}, errors.New("Invalid check interval value (" + intervalStr + "), must be valid INT and no lower than 1")
		}
	}

	return Config{
		HealthAddr:      ":8080",
		WorkInterval:    time.Duration(interval) * time.Second,
		ShutdownTimeout: 15 * time.Second,
		ROSHost:         rosHost,
		ROSUser:         rosUser,
		ROSPass:         rosPass,
		DNSAPIToken:     dnsApiToken,
		DNSZoneID:       dnsZoneId,
		DNSRecordID:     dnsRecordId,
		DNSRecordName:   dnsRecordName,
		RecordTTL:       recordTTL,
	}, nil
}
