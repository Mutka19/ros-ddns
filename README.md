# RouterOS DDNS Daemon (ros-ddnsd)

A lightweight Go-based background service containerized with Docker to keep your DNS records in sync with your MikroTik RouterOS WAN IP.

The daemon polls your MikroTik router's REST API at a configurable interval to find the IP assigned to your DHCP client, caches it, and dynamically updates your specified DNS A Record whenever a change is detected.

## Features

Zero Memory Leak / Efficient Polling: Written in Go with an internal sync cache to prevent redundant API calls to your DNS provider.

Secure Secret Management: Utilizes Docker Secrets to keep sensitive payloads (DNS Provider API tokens and Router OS credentials) completely out of environment variables and shell history.

Health Check Endpoint: Exposes a `:8080/healthz` endpoint for container health monitoring.

Graceful Shutdown: Properly catches `SIGINT` and `SIGTERM` signals to drain connections and close internal routines cleanly.

## Directory Layout

```
.
├── certs/                 # Place your trusted router certificates here (.crt)
├── cmd/
├── secrets/               # Directory for local Docker Secret files
│   ├── dns_api_token.txt
│   ├── ros_user.txt
│   └── ros_pass.txt
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

## Prerequisites

MikroTik RouterOS v7+: Needs the REST API service active and a user account with **read permissions**.

DNS zone: A zone with an existing A Record created that you intend to update.

Docker & Docker Compose installed on your host system.

## Setup & Configuration

### 1. Configure Secrets

Create a `secrets` directory in your project root and populate the following three text files. Make sure not to add trailing newlines or extra spaces to these files.

```
mkdir secrets
echo -n "your_dns_api_token" > secrets/dns_api_token.txt
echo -n "your_routeros_username" > secrets/ros_user.txt
echo -n "your_routeros_password" > secrets/ros_pass.txt
```

### 2. Add Router HTTPS Certificates (if using self-signed certificate)

If your RouterOS REST API uses a self-signed certificate or a local CA and you want to ensure secure TLS validation, place your router's public certificate file (in `.crt` format) inside the `certs` folder.

### 3. Update Environment Variables

Open your `docker-compose.yml` and adjust the variables under the `environment` block to match your networking stack:

```
environment:
  ROS_HOST: 192.168.88.1        # IP address or hostname of your MikroTik Router
  DNS_RECORD_NAME: router.example.com # The target DNS record you want to update
  DNS_ZONE_ID: "your_zone_id"    # Found on your dashboard overview page (if using Cloudflare)
  DNS_RECORD_ID: "your_rec_id"   # DNS internal API ID for this specific A Record
  RECORD_TTL: 60                # DNS TTL in seconds (Minimum: 30)
  CHECK_INTERVAL: 30            # How frequently to poll the router (in seconds)
```

### Usage

Once your variables and secret files are configured, spin up the container using Docker Compose:

```
# Build and run the daemon in the foreground to test the stream
docker compose up --build

\* # Run in detached mode once confirmed working
docker compose up -d
```


### Logs & Monitoring

The daemon outputs structural JSON logs via standard output. You can trace its workflow using:
```
docker compose logs -f
```


Example output stream:

```
{"time":"2026-05-27T10:00:51Z","level":"INFO","msg":"daemon starting","health_addr":":8080"}
{"time":"2026-05-27T10:01:21Z","level":"INFO","msg":"DNS updated","new_ip":"192.0.2.1"}
```


To view the basic container readiness check from the host system, you can hit the health endpoint:
```
curl http://localhost:8080/healthz
# Returns: ok
```

### Curent Limitations
At this time only Cloudflare as a DNS provider is supported, but more DNS providers will be added in the future
