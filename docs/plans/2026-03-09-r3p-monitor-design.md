# r3p-dashboard Design

Standalone Go binary that subscribes to MQTT topics published by r3p-mqtt,
logs all data to SQLite, and serves a web dashboard for live state and
historical time-series visualization.

## Constraints

- Zero external services (no InfluxDB, no Grafana, no Prometheus)
- Single binary, single systemd unit
- LAN-only, no auth
- Data retained forever with automatic downsampling

## Architecture

```
┌─────────────┐    MQTT    ┌──────────────────────────────┐    HTTP   ┌─────────┐
│ r3p-mqtt    │───────────▶│  r3p-dashboard               │◀──────────│ Browser │
│ (BLE→MQTT)  │            │                              │           └─────────┘
└─────────────┘            │  ┌──────────┐  ┌───────────┐ │
                           │  │ MQTT Sub │  │ HTTP/SSE  │ │
       mosquitto           │  └────┬─────┘  └─────┬─────┘ │
      ┌────────┐           │       │              │       │
      │ broker │           │  ┌────▼──────────────▼─────┐ │
      └────────┘           │  │       SQLite (WAL)      │ │
                           │  └─────────────────────────┘ │
                           └──────────────────────────────┘
```

Three goroutines:
1. **MQTT subscriber** - connects to broker, parses values, writes to SQLite
2. **Compactor** - periodic job that downsamples old data
3. **HTTP server** - serves embedded SPA + JSON API + SSE for live updates

## MQTT Topics

All topics under `ecoflow/r3p/` prefix. The monitor strips the prefix and
stores the short name.

### Numeric topics → `readings` table

Scale 100 (2 decimal places, stored as value × 100):
- `ac_input_power`, `ac_output_power`, `dc_input_power`
- `dc12v_output_power`, `usba_output_power`, `usbc_output_power`
- `cell_temperature`, `input_power`, `output_power`
- `battery_level`

Scale 1 (integers, stored as-is):
- `battery_charge_limit_min`, `battery_charge_limit_max`
- `ac_charging_speed`
- `remaining_time_charging`, `remaining_time_discharging`
- `plugged_in_ac` (bool: 1/0)
- `energy_backup` (bool: 1/0)
- `online` (bool: 1/0)

### Event topics → `events` table
- `fault` (JSON payload)

### Data volume
- 2 topics at ~1/sec (ac_input_power, ac_output_power) = ~170K rows/day
- 16 topics changing rarely (minutes/hours) = negligible
- ~2MB/day raw, compacts to almost nothing

## Data Model

```sql
CREATE TABLE readings (
    ts    INTEGER NOT NULL,  -- unix seconds
    topic TEXT    NOT NULL,
    value INTEGER NOT NULL,  -- raw value × scale factor
    PRIMARY KEY (ts, topic)
);

CREATE TABLE readings_5m (
    ts  INTEGER NOT NULL,
    topic TEXT NOT NULL,
    min INTEGER NOT NULL,
    max INTEGER NOT NULL,
    avg INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE readings_1h (
    ts  INTEGER NOT NULL,
    topic TEXT NOT NULL,
    min INTEGER NOT NULL,
    max INTEGER NOT NULL,
    avg INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE readings_1d (
    ts  INTEGER NOT NULL,
    topic TEXT NOT NULL,
    min INTEGER NOT NULL,
    max INTEGER NOT NULL,
    avg INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE events (
    ts      INTEGER NOT NULL,
    type    TEXT    NOT NULL,  -- "fault"
    payload TEXT    NOT NULL,  -- JSON
    PRIMARY KEY (ts, type)
);
```

## Retention & Compaction

Compactor runs once per hour (and on startup to catch up):

| Source       | Target       | Threshold | Group by          |
|-------------|-------------|-----------|-------------------|
| readings    | readings_5m | > 7 days  | (ts / 300) × 300 |
| readings_5m | readings_1h | > 90 days | (ts / 3600) × 3600 |
| readings_1h | readings_1d | > 2 years | (ts / 86400) × 86400 |

Each step: aggregate (min/max/avg), insert into target, delete from source.
All within a single transaction per step. WAL mode for concurrent reads.

## Query Resolution

Frontend requests a time range; backend auto-selects the table:

| Range       | Table       |
|------------|-------------|
| ≤ 6 hours  | readings    |
| ≤ 7 days   | readings_5m |
| ≤ 90 days  | readings_1h |
| > 90 days  | readings_1d |

## HTTP API

```
GET /api/current                        → latest value for all topics
GET /api/history?topic=X&from=T&to=T    → time-series (auto-selects table)
GET /api/topics                         → topic registry (name, scale, unit)
GET /api/events?from=T&to=T             → fault events
SSE /api/stream                         → real-time updates from MQTT
```

## Frontend

Single `index.html` embedded in the binary via `//go:embed`.

Stack:
- **Tailwind CSS 4** (CDN) — dark theme
- **Alpine.js** (CDN) — reactivity for cards, controls, SSE binding
- **uPlot** (CDN) — time-series charts

Layout:
- **Top bar** — title + online status indicator
- **Live cards** — battery %, input/output power, temperature, port states
- **Chart area** — time range selector [6h/24h/7d/30d/1y/all], topic picker, uPlot chart

## Project Structure

```
r3p-dashboard/
├── main.go               # entry, config, wiring
├── go.mod
├── config.go             # MQTT host/port, DB path, listen addr
├── config.json           # runtime config (gitignored)
├── config.example.json
├── mqtt.go               # MQTT subscriber, value parsing, DB writes
├── db.go                 # SQLite setup, insert, query, compaction
├── api.go                # HTTP handlers + SSE
├── topics.go             # topic registry (name, scale, unit, type)
├── web/
│   └── index.html        # Tailwind 4 + Alpine.js + uPlot
├── r3p-dashboard.service   # systemd unit
└── README.md
```

## Config

```json
{
  "mqtt": {
    "host": "localhost",
    "port": 1883,
    "topic_prefix": "ecoflow/r3p"
  },
  "db_path": "database.db",
  "listen": ":8080"
}
```
