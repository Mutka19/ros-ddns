package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
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

func readSecret(name string) (string, error) {
	data, err := os.ReadFile("/run/secrets/" + name)
	if err != nil {
		return "", fmt.Errorf("reading secret %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func setConfig() (Config, error) {
	// Ensure all required variables and secrets are loaded
	rosHost := os.Getenv("ROS_HOST")
	if rosHost == "" {
		return Config{}, errors.New("ROS_HOST is required")
	}

	rosUser, err := readSecret("ros_user")
	if err != nil || rosUser == "" {
		return Config{}, errors.New("ROS_USER secret is required")
	}

	rosPass, err := readSecret("ros_pass")
	if err != nil || rosPass == "" {
		return Config{}, errors.New("ROS_PASS secret is required")
	}

	dnsApiToken, err := readSecret("dns_api_token")
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

type Daemon struct {
	cfg     Config
	cfclt   *cloudflare.Client
	httpclt *http.Client
	ipcache sync.Map
	log     *slog.Logger
	server  *http.Server
	wg      sync.WaitGroup
}

func NewDaemon(cfg Config, log *slog.Logger) *Daemon {
	d := &Daemon{cfg: cfg, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", d.handleHealth)
	d.server = &http.Server{
		Addr:    cfg.HealthAddr,
		Handler: mux,
	}

	d.cfclt = cloudflare.NewClient(option.WithAPIToken(cfg.DNSAPIToken))
	d.httpclt = &http.Client{}

	return d
}

type addrResponse struct {
	Address string `json:"address"`
}

func fetchIP(ctx context.Context, cfg Config, client *http.Client) (net.IP, error) {
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

func updateDNS(ctx context.Context, cfg Config, client *cloudflare.Client, at time.Time, ip string) error {
	_, err := client.DNS.Records.Edit(
		ctx,
		cfg.DNSRecordID,
		dns.RecordEditParams{
			ZoneID: cloudflare.F(cfg.DNSZoneID),
			Body: dns.ARecordParam{
				Content: cloudflare.F(ip),
				Name:    cloudflare.F(cfg.DNSRecordName),
				TTL:     cloudflare.F(dns.TTL(cfg.RecordTTL)),
				Type:    cloudflare.F(dns.ARecordType("A")),
				Comment: cloudflare.F("Last update via ROS ddnsd: " + at.UTC().Format(time.UnixDate)),
			},
		},
	)
	return err
}

// Run blocks until ctx is cancelled, then performs a graceful shutdown.
func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("daemon starting", "health_addr", d.cfg.HealthAddr)

	// Start the health/metrics HTTP server in the background.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.log.Error("health server error", "err", err)
		}
	}()

	// Start the main work loop.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.workLoop(ctx)
	}()

	// Block until the context is cancelled (i.e. a signal was received).
	<-ctx.Done()
	d.log.Info("shutdown signal received, draining...")

	return d.shutdown()
}

func (d *Daemon) workLoop(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.WorkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.log.Info("work loop stopping")
			return
		case t := <-ticker.C:
			if err := d.doWork(ctx, t); err != nil {
				// Log but continue
				d.log.Error("work iteration failed", "err", err)
			}
		}
	}
}

func (d *Daemon) doWork(ctx context.Context, at time.Time) error {
	// Make api call to ROS device API to obtain dhcp client leased IP
	ip, err := fetchIP(ctx, d.cfg, d.httpclt)

	if err != nil {
		return fmt.Errorf("fetching ip: %w", err)
	}

	// Load or Store assigned IP address using cache
	ipstr := ip.String()
	cachedIP, loaded := d.ipcache.LoadOrStore("IP", ipstr)

	// If cached value was loaded from cache AND IP has changed
	if loaded && cachedIP != ipstr {
		d.ipcache.Store("IP", ipstr)
	}

	// Update DNS record via Cloudflare API if leased IP has changed
	if !loaded || cachedIP != ipstr {
		if err := updateDNS(ctx, d.cfg, d.cfclt, at, ipstr); err != nil {
			return fmt.Errorf("updating DNS: %w", err)
		}
		d.log.Info("DNS updated", "new_ip", ipstr)
	}

	return nil
}

// shutdown gracefully stops all components within the configured timeout.
func (d *Daemon) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.ShutdownTimeout)
	defer cancel()

	// Stop accepting new HTTP requests.
	if err := d.server.Shutdown(ctx); err != nil {
		d.log.Error("health server shutdown error", "err", err)
	}

	// Wait for all goroutines with a deadline.
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		d.log.Info("daemon stopped cleanly")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timed out after %s", d.cfg.ShutdownTimeout)
	}
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := setConfig()
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	d := NewDaemon(cfg, log)

	if err := d.Run(ctx); err != nil {
		log.Error("daemon exited with error", "err", err)
		os.Exit(1)
	}
}
