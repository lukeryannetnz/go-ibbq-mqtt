# go-ibbq-mqtt Implementation Plan

This plan is for `go-ibbq-mqtt` at `jaidan@bbq:/home/jaidan/Documents/projects/go-ibbq-mqtt`.

---

## Completed Work

All features below are implemented and running.

- **Systemd service** — `go-ibbq-mqtt@.service` template and `go-ibbq-mqtt.service` single-instance in the repo. Runs as `ibbq` user with `CAP_NET_ADMIN CAP_NET_RAW`, reads config from `EnvironmentFile`, restarts on crash.
- **Reconnection with retry** — `connectWithRetry` in `main.go` survives transient BLE drops. Retry delays: 5s → 10s → 20s → 60s cap. MQTT status published at each state change.
- **Per-device MQTT topics** — `DEVICE_NAME`/`DEVICE_MAC` env vars target a specific device. Topics: `{MQTT_TOPIC}/{DEVICE_NAME}/{subtopic}`.
- **Web status page** — `web.go` serves live dashboard on `WEB_PORT`. Routes: `/api/devices`, `/api/devices/{name}`, `/`. In-memory device state updated by BLE callbacks.

---

## Recommended Commit Before Continuing

> **Commit the current state as: `"functional with one device"`**

The codebase is working with a single device. Everything below is a redesign. Commit now so there is a clean rollback point.

---

## Redesign — Automatic Multi-Device Discovery and Management

### Goal

The service scans for all nearby iBBQ-family devices automatically, connects to each, and manages them independently. Devices are identified by MAC and assigned a generated UID. Users name devices and configure per-device poll intervals through the web UI. Devices unseen for 10 minutes are marked dead.

Observed BLE local-name variants in the field:
- `iBBQ`
- `xBBQ`

Discovery and connection filters must accept both names.

### BLE Constraints (Researched)

The go-ble library version `v0.0.0-20181002102605-e78417b510a3` has the following confirmed behaviour on Linux:

1. **`ble.Connect` cannot be called concurrently.** It internally calls `ble.Scan` to find the target device, then calls `Dial()` which blocks on a single `chMasterConn` channel in the HCI layer. Two simultaneous `ble.Connect` calls will race on that channel. **All initial connections must be made sequentially, one at a time.**

2. **Multiple concurrent LE connections are supported.** Once a connection is established, it lives in an `HCI.conns map[uint16]*Conn`. Subsequent connections (initiated sequentially) add to this map. All connections then operate in parallel independently. A device connected to probe A does not block probe B's data stream.

3. **`ble.Scan` and active connections can coexist** at the BlueZ kernel level. However, go-ble's `Connect()` starts a scan internally each time, so running a discovery scan while a `Connect()` call is in flight will conflict. The safe pattern is: finish all `Connect()` calls first, then scan; or scan first, then connect.

4. **`ble.SetDefaultDevice` is process-wide and not concurrency-safe.** It must be called exactly once before any scan or connect. The `InitBLE()` function using `sync.Once` (described below) enforces this.

5. **iBBQ pushes temperature data continuously** via BLE notifications after `enableRealTimeData()` is called. The device firmware determines push frequency (~1–10 notifications/second). The client cannot request a specific push rate. The "poll interval" in this system therefore means: **how often to publish the latest cached reading to MQTT**, not how often to request data from the device.

---

### Architecture Overview

```
main.go
  ├── InitBLE()                    — one-time BLE hardware init
  ├── Load Config + Registry       — from /var/lib/go-ibbq-mqtt/
  ├── Start MQTT client
  ├── Start web server             — go startWebServer()
  ├── Start DeviceManager          — go dm.Run(ctx)
  │     ├── Initial BLE scan       — scanner.go: ScanForDevices()
  │     ├── Sequential connects    — one ble.Connect at a time
  │     ├── Per-device goroutines  — one goroutine per device, runs forever
  │     ├── Dead checker           — goroutine, fires every 30s
  │     └── Periodic re-scan       — goroutine, fires at ScanIntervalSec
  └── Signal handler → cancel context → graceful shutdown

registry.go      — device config + runtime state, file-backed
scanner.go       — ble.Scan wrapper returning discovered MACs
devicemanager.go — orchestrates scanner, connections, dead detection
web.go           — HTTP server, extended API, editable UI
mqttClient.go    — unchanged except minor topic helper updates
internal/ibbq/   — add InitBLE(); fix realTimeDataReceived sentinel filter
```

---

### Files to Delete / Remove Entirely

| File / Code | Reason |
|-------------|--------|
| `readDeviceConfig()` in `main.go` | Replaced by registry + auto-discovery |
| `readDeviceConfigs()` (planned but not yet written) | Not needed |
| `connectWithRetry()` in `main.go` | Replaced by `DeviceManager` |
| `tryConnect()` in `main.go` | Replaced by `DeviceManager` |
| `deviceConfig` struct in `main.go` | Replaced by `DeviceRecord` in `registry.go` |
| `DEVICE_NAME`, `DEVICE_MAC` env var reads | Discovery replaces manual config |
| `DEVICE_NAMES`, `DEVICE_MACS` env var reads | Never written; do not add |
| Template service `go-ibbq-mqtt@.service` | Single process handles all devices; template no longer needed |
| `DEVICE_NAME` and `DEVICE_MAC` from `/etc/default/go-ibbq-mqtt` | Remove those two lines |

---

### New File: `registry.go`

Holds device configuration (persisted) and runtime state (in-memory). Saved to `/var/lib/go-ibbq-mqtt/registry.json` on every mutation.

```go
// DeviceConfig is the persisted part of a device record.
type DeviceConfig struct {
    MAC             string    `json:"mac"`
    UID             string    `json:"uid"`             // UUID v4 assigned on first discovery
    Name            string    `json:"name"`            // user-assigned label, defaults to last 5 chars of MAC
    PollIntervalSec int       `json:"pollIntervalSec"` // how often to publish to MQTT, default 5
    FirstSeen       time.Time `json:"firstSeen"`
}

// DeviceRecord combines persisted config with runtime state.
type DeviceRecord struct {
    Config       DeviceConfig
    Status       string    // "Connecting" | "Connected" | "Disconnected" | "Dead"
    LastSeen     time.Time // last BLE notification received
    Temperatures []float64 // latest probe readings
    BatteryLevel int
}

// AppConfig is the persisted global configuration.
type AppConfig struct {
    ScanIntervalSec        int `json:"scanIntervalSec"`        // default 300
    DefaultPollIntervalSec int `json:"defaultPollIntervalSec"` // default 5
}

// RegistryFile is the structure written to disk.
type RegistryFile struct {
    Config  AppConfig               `json:"config"`
    Devices map[string]DeviceConfig `json:"devices"` // keyed by MAC
}

// Registry is the in-memory store.
type Registry struct {
    mu      sync.RWMutex
    config  AppConfig
    devices map[string]*DeviceRecord // keyed by MAC
    path    string
}
```

**Methods on `*Registry`:**

- `Load(path string) error` — reads `registry.json`; initialises empty maps if file doesn't exist
- `Save() error` — marshals `DeviceConfig` for all devices + `AppConfig` to `registry.json` atomically (write to `.tmp`, rename)
- `RegisterDevice(mac string) *DeviceRecord` — if MAC not known, creates a new `DeviceConfig` with a UUID v4 UID, default name (last 5 chars of MAC), default poll interval; saves; returns record
- `Get(mac string) *DeviceRecord` — thread-safe lookup
- `All() []*DeviceRecord` — returns snapshot of all records
- `SetName(mac, name string) error` — update name, save
- `SetPollInterval(mac string, sec int) error` — update, save
- `SetStatus(mac, status string)` — in-memory only
- `UpdateReadings(mac string, temps []float64, battery int)` — updates `Temperatures`, `BatteryLevel`, `LastSeen`; in-memory only
- `SetConfig(cfg AppConfig) error` — update global config, save

UUID generation: use `crypto/rand` to generate a UUID v4. No external dependency needed:

```go
func newUID() string {
    b := make([]byte, 16)
    _, _ = rand.Read(b)
    b[6] = (b[6] & 0x0f) | 0x40
    b[8] = (b[8] & 0x3f) | 0x80
    return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
```

---

### New File: `scanner.go`

Wraps `ble.Scan` to discover all iBBQ devices within a time window.

```go
// ScanForDevices scans for all connectable iBBQ-family advertisements for the given
// duration and returns the unique MAC addresses found.
// ble.Scan blocks until the context deadline, then returns context.DeadlineExceeded
// which is treated as success (scan completed normally).
func ScanForDevices(ctx context.Context, duration time.Duration) ([]string, error) {
    seen := make(map[string]bool)
    var mu sync.Mutex

    scanCtx, cancel := context.WithTimeout(ctx, duration)
    defer cancel()

    err := ble.Scan(scanCtx, false,
        func(a ble.Advertisement) {
            mac := strings.ToUpper(a.Addr().String())
            mu.Lock()
            seen[mac] = true
            mu.Unlock()
        },
        func(a ble.Advertisement) bool {
            return ibbq.IsSupportedDeviceName(a.LocalName()) && a.Connectable()
        },
    )

    if err != nil && !errors.Is(err, context.DeadlineExceeded) {
        return nil, err
    }

    macs := make([]string, 0, len(seen))
    for mac := range seen {
        macs = append(macs, mac)
    }
    return macs, nil
}
```

**Note:** `ble.Scan` must not be running when `ble.Connect` is called. `ScanForDevices` returns only after the scan window closes. The caller must not call `ble.Connect` until `ScanForDevices` returns.

---

### New File: `devicemanager.go`

Orchestrates discovery, sequential connecting, per-device goroutines, dead detection, and periodic re-scanning.

```go
type DeviceManager struct {
    registry *Registry
    mc       MqttClient

    mu      sync.Mutex
    cancels map[string]context.CancelFunc // MAC -> cancel for that device's goroutine
}

func NewDeviceManager(registry *Registry, mc MqttClient) *DeviceManager {
    return &DeviceManager{
        registry: registry,
        mc:       mc,
        cancels:  make(map[string]context.CancelFunc),
    }
}
```

**`(dm *DeviceManager) Run(ctx context.Context)`**

Entry point called from `main()` in a goroutine.

```
1. Run initial scan: ScanForDevices(ctx, 30s)
2. For each discovered MAC: registry.RegisterDevice(mac)
3. Connect to each new device sequentially (see connectSequential below)
4. Start dead-checker goroutine
5. Start periodic-rescan goroutine
6. Block on ctx.Done()
```

**`(dm *DeviceManager) connectSequential(ctx context.Context, macs []string)`**

Because `ble.Connect` cannot be called concurrently, this connects to devices one at a time. Each connection blocks until established (or fails) before moving to the next. Once connected, a per-device goroutine is launched and the function moves on.

```go
func (dm *DeviceManager) connectSequential(ctx context.Context, macs []string) {
    for _, mac := range macs {
        select {
        case <-ctx.Done():
            return
        default:
        }

        if dm.isManaged(mac) {
            continue // already has a running goroutine
        }

        // Block here until connected or connect timeout fires.
        // This ensures only one ble.Connect call is in flight at a time.
        done := make(chan struct{})
        if err := dm.tryConnect(ctx, mac, done); err != nil {
            logger.Error("Initial connect failed", "mac", mac, "err", err)
            dm.registry.SetStatus(mac, "Disconnected")
            // Still launch a reconnect goroutine so it will retry.
        }

        // Launch the device lifecycle goroutine.
        devCtx, cancel := context.WithCancel(ctx)
        dm.mu.Lock()
        dm.cancels[mac] = cancel
        dm.mu.Unlock()

        go dm.manageDevice(devCtx, mac, done)
    }
}
```

**`(dm *DeviceManager) manageDevice(ctx context.Context, mac string, initialDone chan struct{})`**

Long-running goroutine per device. Waits for disconnect, then reconnects.

```go
func (dm *DeviceManager) manageDevice(ctx context.Context, mac string, initialDone chan struct{}) {
    defer dm.removeCancelEntry(mac)

    done := initialDone

    for {
        // Wait for disconnect or context cancel.
        select {
        case <-ctx.Done():
            return
        case <-done:
            logger.Info("Device disconnected, will reconnect", "mac", mac)
            dm.registry.SetStatus(mac, "Disconnected")
        }

        // Exponential backoff reconnect loop.
        attempts := 0
        delays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}
        for {
            select {
            case <-ctx.Done():
                return
            default:
            }

            done = make(chan struct{})
            if err := dm.tryConnect(ctx, mac, done); err == nil {
                attempts = 0
                break // connected, go back to waiting for disconnect
            }

            attempts++
            delay := 60 * time.Second
            if attempts-1 < len(delays) {
                delay = delays[attempts-1]
            }

            logger.Error("Reconnect failed", "mac", mac, "attempt", attempts, "retryIn", delay)
            select {
            case <-ctx.Done():
                return
            case <-time.After(delay):
            }
        }
    }
}
```

**`(dm *DeviceManager) tryConnect(ctx context.Context, mac string, done chan struct{}) error`**

Single connection attempt. Blocks until connected or timeout.

```go
func (dm *DeviceManager) tryConnect(ctx context.Context, mac string, done chan struct{}) error {
    record := dm.registry.Get(mac)

    config, _ := ibbq.NewConfiguration(60*time.Second, 5*time.Minute)
    config.TargetMAC = mac

    bbq, err := ibbq.NewIbbq(ctx, config,
        func() { close(done) },
        func(temps []float64) { dm.onTemperature(mac, temps) },
        func(level int)       { dm.onBattery(mac, level) },
        func(s ibbq.Status)   { dm.onStatus(mac, s) },
    )
    if err != nil {
        return err
    }

    dm.registry.SetStatus(mac, "Connecting")
    _ = record // suppress unused warning; name is used in MQTT publish helpers
    return bbq.Connect()
}
```

**Callbacks** (called from device goroutines):

```go
func (dm *DeviceManager) onTemperature(mac string, temps []float64) {
    dm.registry.UpdateReadings(mac, temps, -1) // -1 = no battery update
    record := dm.registry.Get(mac)
    if record == nil {
        return
    }
    // Throttle MQTT publishes to the configured poll interval.
    // The registry tracks lastPublished per device; skip if too soon.
    if time.Since(record.lastPublished) < time.Duration(record.Config.PollIntervalSec)*time.Second {
        return
    }
    record.lastPublished = time.Now()

    name := record.Config.Name
    t := &temperature{temps}
    dm.mc.Pub(name, "temperatures", t.toJson())
}

func (dm *DeviceManager) onBattery(mac string, level int) {
    dm.registry.UpdateReadings(mac, nil, level)
    record := dm.registry.Get(mac)
    if record == nil { return }
    b := &batteryLevel{level}
    dm.mc.Pub(record.Config.Name, "batterylevel", b.toJson())
}

func (dm *DeviceManager) onStatus(mac string, s ibbq.Status) {
    dm.registry.SetStatus(mac, string(s))
    record := dm.registry.Get(mac)
    if record == nil { return }
    publishStatus(record.Config.Name, string(s))
}
```

**`lastPublished` field**: Add to `DeviceRecord` (not persisted):
```go
lastPublished time.Time
```

**Dead-checker goroutine:**

```go
func (dm *DeviceManager) runDeadChecker(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, record := range dm.registry.All() {
                if record.Status == "Connected" &&
                    !record.LastSeen.IsZero() &&
                    time.Since(record.LastSeen) > 10*time.Minute {
                    logger.Warn("Device has not sent data in 10 minutes, marking dead", "mac", record.Config.MAC)
                    dm.registry.SetStatus(record.Config.MAC, "Dead")
                    // Cancel the device goroutine — it will stop retrying.
                    dm.mu.Lock()
                    if cancel, ok := dm.cancels[record.Config.MAC]; ok {
                        cancel()
                        delete(dm.cancels, record.Config.MAC)
                    }
                    dm.mu.Unlock()
                    publishStatus(record.Config.Name, "Dead")
                }
            }
        }
    }
}
```

**Dead device recovery**: A dead device's goroutine has been cancelled. It will not reconnect on its own. The periodic re-scan (below) may rediscover it, which calls `connectSequential` again and launches a new goroutine.

**Periodic re-scan goroutine:**

```go
func (dm *DeviceManager) runPeriodicScan(ctx context.Context) {
    interval := time.Duration(dm.registry.Config().ScanIntervalSec) * time.Second
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            logger.Info("Running periodic BLE scan")
            macs, err := ScanForDevices(ctx, 30*time.Second)
            if err != nil {
                logger.Error("Periodic scan failed", "err", err)
                continue
            }
            for _, mac := range macs {
                dm.registry.RegisterDevice(mac)
            }
            dm.connectSequential(ctx, macs)
        }
    }
}
```

**Important:** `ScanForDevices` must not run while a `ble.Connect` call is in flight. The periodic scan only starts after all initial connects complete, and `connectSequential` is synchronous — it won't return until all new connections are made. The re-scan goroutine and connect phase are therefore naturally serialised (both run in the same goroutine).

**Manual scan trigger** (from web UI): `POST /api/scan` calls `dm.TriggerScan()` which sends to a `rescanCh chan struct{}`. The main `runPeriodicScan` goroutine listens on both the ticker and `rescanCh`.

---

### Updated `internal/ibbq/ibbq.go`

**Two changes required:**

**1. Add `InitBLE()` and remove BLE init from `NewIbbq`:**

```go
var (
    bleInitOnce sync.Once
    bleInitErr  error
)

// InitBLE initialises the BLE hardware. Must be called once before any
// scan or connect. Subsequent calls are no-ops and return the first result.
func InitBLE() error {
    bleInitOnce.Do(func() {
        d, err := NewDevice("default")
        if err != nil {
            bleInitErr = err
            return
        }
        ble.SetDefaultDevice(d)
    })
    return bleInitErr
}
```

Remove lines 63–64 from `NewIbbq` (the `NewDevice` + `ble.SetDefaultDevice` calls). The `Ibbq.device` field is then unused — remove it from the struct and the constructor call on line 65.

**2. Fix `realTimeDataReceived` to filter the no-probe sentinel:**

Replace the existing loop in `realTimeDataReceived` (`ibbq.go:194–206`):

```go
func (ibbq *Ibbq) realTimeDataReceived() ble.NotificationHandler {
    return func(data []byte) {
        logger.Debug("received real-time data", hex.EncodeToString(data))
        var probeData []float64
        for i := 0; i+1 < len(data); i += 2 {
            raw := binary.LittleEndian.Uint16(data[i : i+2])
            if raw == 0xFFF6 { // sentinel for empty probe slot
                continue
            }
            probeData = append(probeData, float64(raw)/10)
        }
        if len(probeData) > 0 {
            go ibbq.temperatureReceivedHandler(probeData)
        }
    }
}
```

---

### Updated `web.go`

Full rewrite. The web layer now reads from and writes to the `Registry` via a reference passed at startup.

**New API routes:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/devices` | All device records (config + runtime state) |
| `GET` | `/api/devices/{mac}` | Single device |
| `PUT` | `/api/devices/{mac}/name` | Set name. Body: `{"name":"Left Grill"}` |
| `PUT` | `/api/devices/{mac}/poll` | Set poll interval. Body: `{"intervalSec":10}` |
| `GET` | `/api/config` | Global app config |
| `PUT` | `/api/config` | Update global config. Body: `{"scanIntervalSec":300,"defaultPollIntervalSec":5}` |
| `POST` | `/api/scan` | Trigger immediate BLE scan |
| `GET` | `/` | HTML UI |

**Remove:**
- `deviceStates` global map and `deviceStatesMu` — replaced by `Registry`
- `getOrCreateDeviceState()` — replaced by `registry.RegisterDevice()` / `registry.Get()`
- `handleDevice()` by device name — now by MAC address
- The old `handleDeviceList` / `handleDevice` implementations

**`startWebServer` signature change:**

```go
func startWebServer(port string, registry *Registry, dm *DeviceManager)
```

**JSON response shape for a device:**

```json
{
  "mac": "AA:BB:CC:DD:EE:FF",
  "uid": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Left Grill",
  "pollIntervalSec": 5,
  "firstSeen": "2024-01-01T12:00:00Z",
  "status": "Connected",
  "lastSeen": "2024-01-01T15:32:00Z",
  "temperatures": [22.6, 180.3],
  "batteryLevel": 82
}
```

**Web UI changes:**

Replace the current device card layout with an editable table:

- One row per known device (including disconnected and dead ones)
- Columns: Name (editable `<input type="text">`), MAC (read-only), UID (read-only, small text), Status, Temperatures, Battery, Last Seen, Poll Interval (editable `<input type="number" min="1">`), Save button
- Save button calls `PUT /api/devices/{mac}/name` and `PUT /api/devices/{mac}/poll` via `fetch`
- Global config section below the table: scan interval (editable), default poll interval (editable), Save, Scan Now button
- Page auto-refreshes every 5 seconds (existing behaviour)
- Status colour coding: Connected = green, Disconnected = orange, Dead = red, Connecting = grey

---

### Updated `main.go`

Remove all single-device connection code. `main()` becomes:

```go
func main() {
    // ... ascii banner ...
    configureEnv()

    if err := ibbq.InitBLE(); err != nil {
        logger.Fatal("BLE init failed", "err", err)
    }

    registry := &Registry{}
    if err := registry.Load("/var/lib/go-ibbq-mqtt/registry.json"); err != nil {
        logger.Fatal("Failed to load registry", "err", err)
    }

    ctx1, cancel := context.WithCancel(context.Background())
    defer cancel()
    registerInterruptHandler(cancel, ctx1)
    ctx := ble.WithSigHandler(ctx1, cancel)

    mc.Init()

    port := os.Getenv("WEB_PORT")
    if port == "" {
        port = "8080"
    }

    dm := NewDeviceManager(registry, mc)
    go startWebServer(port, registry, dm)
    go dm.Run(ctx)

    <-ctx.Done()
    logger.Info("Exiting")
}
```

Remove these functions entirely from `main.go`:
- `readDeviceConfig()`
- `connectWithRetry()`
- `tryConnect()`
- `temperatureReceived()` — moved into `DeviceManager.onTemperature()`
- `batteryLevelReceived()` — moved into `DeviceManager.onBattery()`
- `statusUpdated()` — moved into `DeviceManager.onStatus()`
- `deviceConfig` struct

---

### Updated `/etc/default/go-ibbq-mqtt`

Remove `DEVICE_NAME` and `DEVICE_MAC`. These are no longer used.

```env
MQTT_SERVER=tcp://localhost:1883
MQTT_TOPIC=ibbq
WEB_PORT=8080
LOGXI=*=INF
```

Remove `go-ibbq-mqtt@.service` from the repo (the template unit). A single process now manages all devices.

---

### Persistence Location

The service `WorkingDirectory` is `/var/lib/go-ibbq-mqtt` and runs as user `ibbq`. The registry file lives at `/var/lib/go-ibbq-mqtt/registry.json`. This directory already exists per the service file. The `ibbq` user must have write permission to it:

```bash
sudo chown ibbq:ibbq /var/lib/go-ibbq-mqtt
```

---

### MQTT Topic Change

Device name is now user-assigned in the registry (not `DEVICE_NAME` env var). MQTT topics use the registry name:

```
ibbq/{name}/temperatures   → {"Temperatures":[t1, t2, ...]}
ibbq/{name}/batterylevel   → {"BatteryLevel": n}
ibbq/{name}/status         → {"Status":"...", "Timestamp":"..."}
```

If name has not been set by the user, the default is the last 5 characters of the MAC (e.g. `EE:FF`), giving topics like `ibbq/EE:FF/temperatures`. Users should assign real names via the web UI before relying on MQTT topics downstream (e.g. in Node-RED).

---

### Files Summary

| File | Action |
|------|--------|
| `main.go` | Rewrite: remove all single-device code; call `InitBLE`, load registry, start DM |
| `internal/ibbq/ibbq.go` | Add `InitBLE()`; remove device init from `NewIbbq`; fix `realTimeDataReceived` sentinel |
| `web.go` | Rewrite: use registry; new API routes; editable table UI |
| `registry.go` | **New**: device config + state store, file persistence |
| `scanner.go` | **New**: `ScanForDevices()` wrapper |
| `devicemanager.go` | **New**: scan orchestration, sequential connect, per-device goroutines, dead detection |
| `mqttClient.go` | No changes |
| `temperature.go` | No changes |
| `batteryLevel.go` | No changes |
| `status.go` | No changes |
| `signals.go` | No changes |
| `go-ibbq-mqtt@.service` | **Delete** — single process replaces template units |
| `/etc/default/go-ibbq-mqtt` | Remove `DEVICE_NAME` and `DEVICE_MAC` lines |

---

### Build and Deploy

```bash
# Compile on Pi
ssh jaidan@bbq
cd /home/jaidan/Documents/projects/go-ibbq-mqtt
go build -o go-ibbq-mqtt .
sudo systemctl restart go-ibbq-mqtt

# Cross-compile from x86 Linux
GOOS=linux GOARCH=arm GOARM=6 go build -o go-ibbq-mqtt .
scp go-ibbq-mqtt jaidan@bbq:/home/jaidan/Documents/projects/go-ibbq-mqtt/
ssh jaidan@bbq sudo systemctl restart go-ibbq-mqtt
```

---

### Acceptance Criteria

- On startup, the service scans for 30 seconds and connects to every supported iBBQ-family device it finds, including `iBBQ` and `xBBQ` variants.
- Each device appears in the web UI table with MAC, UID, status, temperatures, battery, and last seen.
- A user can set a name and poll interval per device via the web UI; changes persist across restarts.
- MQTT topics use the user-assigned name.
- A device that sends no data for 10 minutes is marked Dead in the UI and MQTT status; its goroutine is stopped.
- A dead device rediscovered on the next scan is reconnected automatically.
- The manual Scan button in the UI triggers an immediate BLE scan for new devices.
- All temperatures returned by the BLE notification are real probe readings; empty probe slots (sentinel `0xFFF6`) are excluded.
- The service runs as a single process; the template service unit is removed.
