# r3p-dashboard

Standalone Go binary that subscribes to MQTT topics published by [r3p-mqtt](https://github.com/tectiv3/r3p-mqtt), logs all data to SQLite, and serves a web dashboard for live state and historical time-series visualization.

<img width="800" src="https://github.com/user-attachments/assets/71d6685b-7d35-439e-90e5-e85eb8a7b8d2" />


## Features

- Real-time monitoring via SSE (Server-Sent Events)
- Historical time-series charts with automatic resolution selection
- Automatic data downsampling (raw → 5min → 1h → 1d)
- Dark/light/system theme support
- Configurable metric and chart visibility (saved in browser localStorage)
- Zero external services — single binary, SQLite storage
- Embedded web UI — no separate frontend build step

## Quick Start

```bash
# Build
go build -o r3p-dashboard

# Configure
cp config.example.json config.json
# Edit config.json with your MQTT broker address

# Run
./r3p-dashboard
```

Open `http://localhost:8080` (or whatever port you configured).

## Configuration

```json
{
  "mqtt": {
    "host": "192.168.1.105",
    "port": 1883,
    "topic_prefix": "ecoflow/r3p"
  },
  "db_path": "database.db",
  "listen": ":8080"
}
```

Pass a custom config path as the first argument: `./r3p-dashboard /path/to/config.json`

## API

| Endpoint | Description |
|----------|-------------|
| `GET /api/current` | Latest value for all topics |
| `GET /api/history?topic=X&from=T&to=T` | Time-series (auto-selects resolution) |
| `GET /api/topics` | Topic registry (name, scale, unit) |
| `GET /api/events?from=T&to=T` | Fault events |
| `GET /api/stream` | SSE real-time updates |

## Data Retention

| Age | Resolution |
|-----|-----------|
| < 7 days | Raw (per-second) |
| 7–90 days | 5-minute aggregates |
| 90 days–2 years | 1-hour aggregates |
| > 2 years | 1-day aggregates |

## Deployment

```bash
# Copy binary and config
sudo mkdir -p /opt/r3p-dashboard
sudo cp r3p-dashboard config.json /opt/r3p-dashboard/

# Install systemd service
sudo cp r3p-dashboard.service /etc/systemd/system/
sudo systemctl enable --now r3p-dashboard
```

## License

MIT
