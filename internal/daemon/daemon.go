package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"ros-ddns/internal/config"
	"ros-ddns/internal/dns"
	"ros-ddns/internal/util/router"
	"sync"
	"time"
)

type Daemon struct {
	cfg      config.Config
	provider dns.Provider
	httpclt  *http.Client
	ipcache  sync.Map
	log      *slog.Logger
	server   *http.Server
	wg       sync.WaitGroup
}

func NewDaemon(cfg config.Config, log *slog.Logger) (*Daemon, error) {
	d := &Daemon{cfg: cfg, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", d.handleHealth)
	d.server = &http.Server{
		Addr:    cfg.HealthAddr,
		Handler: mux,
	}

	// Constructs DNS provider based on PROVIDER environment variable
	var err error
	d.provider, err = dns.NewProvider()

	// todo: Add error handling
	if err != nil {
		return nil, err
	}

	d.httpclt = &http.Client{}

	return d, nil
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
	ip, err := router.FetchIP(ctx, d.cfg, d.httpclt)

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
		if err := d.provider.Update(ctx, ipstr, at); err != nil {
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
