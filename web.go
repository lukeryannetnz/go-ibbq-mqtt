package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type webServer struct {
	registry *Registry
	dm       *DeviceManager
}

type deviceResponse struct {
	MAC             string    `json:"mac"`
	UID             string    `json:"uid"`
	Name            string    `json:"name"`
	PollIntervalSec int       `json:"pollIntervalSec"`
	FirstSeen       time.Time `json:"firstSeen"`
	Status          string    `json:"status"`
	LastSeen        time.Time `json:"lastSeen"`
	Temperatures    []float64 `json:"temperatures"`
	BatteryLevel    int       `json:"batteryLevel"`
}

func toDeviceResponse(record *DeviceRecord) deviceResponse {
	return deviceResponse{
		MAC:             record.Config.MAC,
		UID:             record.Config.UID,
		Name:            record.Config.Name,
		PollIntervalSec: record.Config.PollIntervalSec,
		FirstSeen:       record.Config.FirstSeen,
		Status:          record.Status,
		LastSeen:        record.LastSeen,
		Temperatures:    append([]float64(nil), record.Temperatures...),
		BatteryLevel:    record.BatteryLevel,
	}
}

func startWebServer(port string, registry *Registry, dm *DeviceManager) {
	server := &webServer{registry: registry, dm: dm}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices", server.handleDevices)
	mux.HandleFunc("/api/devices/", server.handleDevice)
	mux.HandleFunc("/api/trends", server.handleTrends)
	mux.HandleFunc("/api/config", server.handleConfig)
	mux.HandleFunc("/api/scan", server.handleScan)
	mux.HandleFunc("/", server.handleIndex)

	addr := fmt.Sprintf(":%s", port)
	logger.Info("Starting web server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("Web server error", "err", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *webServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records := s.registry.All()
	resp := make([]deviceResponse, 0, len(records))
	for _, record := range records {
		resp = append(resp, toDeviceResponse(record))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *webServer) handleDevice(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}

	mac := normalizeMAC(parts[0])
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		record := s.registry.Get(mac)
		if record == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, toDeviceResponse(record))
		return
	}

	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch parts[1] {
	case "name":
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := s.registry.SetName(mac, body.Name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	case "poll":
		var body struct {
			IntervalSec int `json:"intervalSec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := s.registry.SetPollInterval(mac, body.IntervalSec); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		http.NotFound(w, r)
		return
	}

	record := s.registry.Get(mac)
	if record == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, toDeviceResponse(record))
}

func (s *webServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.registry.Config())
	case http.MethodPut:
		var cfg AppConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := s.registry.SetConfig(cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.registry.Config())
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *webServer) handleTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.registry.TrendSeries())
}

func (s *webServer) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.dm.TriggerScan()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scan requested"})
}

var indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>iBBQ Monitor</title>
  <style>
    body { font-family: monospace; margin: 0; padding: 24px; background: #f5f1e8; color: #1d1d1d; }
    h1, h2 { margin: 0 0 12px 0; }
    .panel { background: #fffaf0; border: 1px solid #d3c7b8; border-radius: 10px; padding: 16px; margin-bottom: 20px; box-shadow: 0 3px 10px rgba(0,0,0,0.06); }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 10px 8px; border-bottom: 1px solid #e3d9cc; vertical-align: top; text-align: left; }
    th { font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; color: #665b4d; }
    input[type="text"], input[type="number"] { width: 100%; box-sizing: border-box; padding: 6px 8px; border: 1px solid #c9bcae; border-radius: 6px; background: #fff; }
    button { padding: 7px 12px; border: 0; border-radius: 6px; background: #3d6b52; color: white; cursor: pointer; }
    button.secondary { background: #7e5c3f; }
    button:disabled { opacity: 0.5; cursor: wait; }
    .status { font-weight: bold; }
    .status-connected { color: #2e7d32; }
    .status-disconnected { color: #b26a00; }
    .status-dead { color: #b3261e; }
    .status-connecting { color: #5f6368; }
    .small { color: #6c6257; font-size: 12px; }
    .row-actions { display: flex; gap: 8px; align-items: center; }
    .inline { display: flex; gap: 12px; flex-wrap: wrap; align-items: end; }
    .field { min-width: 180px; }
    .temps { white-space: nowrap; }
    .chart-wrap { overflow-x: auto; }
    .legend { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 12px; font-size: 12px; color: #4f463d; }
    .legend-item { display: inline-flex; align-items: center; gap: 6px; }
    .legend-swatch { width: 12px; height: 12px; border-radius: 999px; }
    svg { width: 100%; min-width: 960px; height: 340px; background: #fff; border: 1px solid #e3d9cc; border-radius: 8px; }
    .axis { stroke: #b7aa9c; stroke-width: 1; }
    .grid { stroke: #ece3d8; stroke-width: 1; }
    .axis-label { fill: #6c6257; font-size: 11px; }
  </style>
</head>
<body>
  <div class="panel">
    <h1>iBBQ Monitor</h1>
    <p class="small">Auto-refreshes every 5 seconds. Names and poll intervals are persisted in the registry.</p>
  </div>

  <div class="panel">
    <h2>Global Config</h2>
    <div class="inline">
      <label class="field">Scan interval (sec)<br><input id="scanIntervalSec" type="number" min="1"></label>
      <label class="field">Default poll interval (sec)<br><input id="defaultPollIntervalSec" type="number" min="1"></label>
      <div class="row-actions">
        <button id="saveConfigBtn" onclick="saveConfig()">Save Config</button>
        <button class="secondary" id="scanBtn" onclick="triggerScan()">Scan Now</button>
      </div>
    </div>
  </div>

  <div class="panel">
    <h2>Devices</h2>
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>MAC</th>
          <th>UID</th>
          <th>Status</th>
          <th>Temperatures</th>
          <th>Battery</th>
          <th>Last Seen</th>
          <th>Poll Interval</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody id="deviceRows">
        <tr><td colspan="9">Loading...</td></tr>
      </tbody>
    </table>
  </div>

  <div class="panel">
    <h2>Temperature Trends</h2>
    <p class="small">All valid temperature samples from the last 12 hours. 0C and 6553.5C channels are ignored.</p>
    <div class="chart-wrap">
      <svg id="trendChart" viewBox="0 0 1100 340" preserveAspectRatio="none"></svg>
    </div>
    <div class="legend" id="trendLegend"></div>
  </div>

  <script>
    const chartPalette = ['#c8553d', '#2f6c8f', '#5a7d2b', '#8b4ea8', '#d18f00', '#00897b', '#7a5230', '#5c6bc0'];

    async function loadConfig() {
      const res = await fetch('/api/config');
      const cfg = await res.json();
      document.getElementById('scanIntervalSec').value = cfg.scanIntervalSec || 300;
      document.getElementById('defaultPollIntervalSec').value = cfg.defaultPollIntervalSec || 5;
    }

    function statusClass(status) {
      const value = (status || '').toLowerCase();
      if (value === 'connected') return 'status-connected';
      if (value === 'disconnected') return 'status-disconnected';
      if (value === 'dead') return 'status-dead';
      return 'status-connecting';
    }

    function escapeHtml(value) {
      return String(value ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
    }

    function collectDeviceDrafts() {
      const drafts = {};
      document.querySelectorAll('#deviceRows input[data-mac][data-role]').forEach(input => {
        const mac = input.dataset.mac;
        if (!drafts[mac]) drafts[mac] = {};
        drafts[mac][input.dataset.role] = input.value;
      });
      return drafts;
    }

    function getActiveInputState() {
      const active = document.activeElement;
      if (!active || !active.dataset || !active.dataset.mac || !active.dataset.role) {
        return null;
      }
      return {
        mac: active.dataset.mac,
        role: active.dataset.role,
        start: typeof active.selectionStart === 'number' ? active.selectionStart : null,
        end: typeof active.selectionEnd === 'number' ? active.selectionEnd : null
      };
    }

    function restoreActiveInput(state) {
      if (!state) return;
      const selector = '#deviceRows input[data-mac="' + CSS.escape(state.mac) + '"][data-role="' + CSS.escape(state.role) + '"]';
      const input = document.querySelector(selector);
      if (!input) return;
      input.focus();
      if (state.start !== null && state.end !== null) {
        input.setSelectionRange(state.start, state.end);
      }
    }

    function renderDevices(devices) {
      const tbody = document.getElementById('deviceRows');
      const drafts = collectDeviceDrafts();
      const activeState = getActiveInputState();
      if (!devices.length) {
        tbody.innerHTML = '<tr><td colspan="9">No devices known yet. Run a scan and wait for discovery.</td></tr>';
        return;
      }

      tbody.innerHTML = devices.map((device, index) => {
        const draft = drafts[device.mac] || {};
        const nameValue = draft.name ?? device.name;
        const pollValue = draft.poll ?? device.pollIntervalSec;
        const temps = (device.temperatures || []).length
          ? device.temperatures.map((t, i) => 'T' + (i + 1) + ': ' + Number(t).toFixed(1) + '°C').join('<br>')
          : '--';
        const battery = device.batteryLevel >= 0 ? device.batteryLevel + '%' : '--';
        const lastSeen = device.lastSeen ? new Date(device.lastSeen).toLocaleString() : '--';
        return '<tr>' +
          '<td><input type="text" id="name-' + index + '" data-mac="' + escapeHtml(device.mac) + '" data-role="name" value="' + escapeHtml(nameValue) + '"></td>' +
          '<td>' + escapeHtml(device.mac) + '</td>' +
          '<td class="small">' + escapeHtml(device.uid) + '</td>' +
          '<td><span class="status ' + statusClass(device.status) + '">' + escapeHtml(device.status || '--') + '</span></td>' +
          '<td class="temps">' + temps + '</td>' +
          '<td>' + escapeHtml(battery) + '</td>' +
          '<td>' + escapeHtml(lastSeen) + '</td>' +
          '<td><input type="number" min="1" id="poll-' + index + '" data-mac="' + escapeHtml(device.mac) + '" data-role="poll" value="' + escapeHtml(pollValue) + '"></td>' +
          '<td><button onclick="saveDevice(\'' + encodeURIComponent(device.mac) + '\', ' + index + ')">Save</button></td>' +
          '</tr>';
      }).join('');
      restoreActiveInput(activeState);
    }

    async function loadDevices() {
      const res = await fetch('/api/devices');
      const devices = await res.json();
      renderDevices(devices);
    }

    function seriesPath(points, minTime, maxTime, minValue, maxValue, dims) {
      const timeSpan = Math.max(maxTime - minTime, 1);
      const valueSpan = Math.max(maxValue - minValue, 1);
      return points.map((point, index) => {
        const x = dims.left + ((new Date(point.timestamp).getTime() - minTime) / timeSpan) * dims.width;
        const y = dims.top + dims.height - ((point.value - minValue) / valueSpan) * dims.height;
        return (index === 0 ? 'M' : 'L') + x.toFixed(2) + ' ' + y.toFixed(2);
      }).join(' ');
    }

    function renderTrendChart(series) {
      const svg = document.getElementById('trendChart');
      const legend = document.getElementById('trendLegend');
      if (!series.length || !series.some(item => item.points && item.points.length)) {
        svg.innerHTML = '<text x="40" y="40" class="axis-label">No valid trend data yet.</text>';
        legend.innerHTML = '';
        return;
      }

      const flattened = series.flatMap(item => item.points.map(point => ({
        timestamp: new Date(point.timestamp).getTime(),
        value: point.value
      })));
      const minTime = Math.min(...flattened.map(point => point.timestamp));
      const maxTime = Math.max(...flattened.map(point => point.timestamp));
      const minValue = Math.min(...flattened.map(point => point.value));
      const maxValue = Math.max(...flattened.map(point => point.value));
      const dims = { left: 54, top: 18, width: 1010, height: 270 };

      const gridLines = [];
      for (let i = 0; i < 5; i++) {
        const y = dims.top + (dims.height / 4) * i;
        const value = (maxValue - ((maxValue - minValue) / 4) * i).toFixed(1);
        gridLines.push('<line class="grid" x1="' + dims.left + '" y1="' + y + '" x2="' + (dims.left + dims.width) + '" y2="' + y + '"></line>');
        gridLines.push('<text class="axis-label" x="6" y="' + (y + 4) + '">' + value + '°C</text>');
      }
      for (let i = 0; i < 6; i++) {
        const x = dims.left + (dims.width / 5) * i;
        const timestamp = new Date(minTime + ((maxTime - minTime) / 5) * i);
        gridLines.push('<line class="grid" x1="' + x + '" y1="' + dims.top + '" x2="' + x + '" y2="' + (dims.top + dims.height) + '"></line>');
        gridLines.push('<text class="axis-label" x="' + (x - 22) + '" y="316">' + timestamp.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) + '</text>');
      }

      const lines = series.map((item, index) => {
        const color = chartPalette[index % chartPalette.length];
        const path = seriesPath(item.points, minTime, maxTime, minValue, maxValue, dims);
        return '<path d="' + path + '" fill="none" stroke="' + color + '" stroke-width="2"></path>';
      }).join('');

      svg.innerHTML =
        gridLines.join('') +
        '<line class="axis" x1="' + dims.left + '" y1="' + (dims.top + dims.height) + '" x2="' + (dims.left + dims.width) + '" y2="' + (dims.top + dims.height) + '"></line>' +
        '<line class="axis" x1="' + dims.left + '" y1="' + dims.top + '" x2="' + dims.left + '" y2="' + (dims.top + dims.height) + '"></line>' +
        lines;

      legend.innerHTML = series.map((item, index) => {
        const color = chartPalette[index % chartPalette.length];
        return '<span class="legend-item"><span class="legend-swatch" style="background:' + color + '"></span>' + escapeHtml(item.name) + '</span>';
      }).join('');
    }

    async function loadTrends() {
      const res = await fetch('/api/trends');
      const series = await res.json();
      renderTrendChart(series);
    }

    async function saveDevice(encodedMac, index) {
      const mac = decodeURIComponent(encodedMac);
      const name = document.getElementById('name-' + index).value;
      const intervalSec = Number(document.getElementById('poll-' + index).value);

      const nameRes = await fetch('/api/devices/' + encodeURIComponent(mac) + '/name', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name })
      });
      if (!nameRes.ok) {
        alert('Failed to save device name');
        return;
      }

      const pollRes = await fetch('/api/devices/' + encodeURIComponent(mac) + '/poll', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ intervalSec })
      });
      if (!pollRes.ok) {
        alert('Failed to save poll interval');
        return;
      }

      await loadDevices();
    }

    async function saveConfig() {
      const button = document.getElementById('saveConfigBtn');
      button.disabled = true;
      try {
        const payload = {
          scanIntervalSec: Number(document.getElementById('scanIntervalSec').value),
          defaultPollIntervalSec: Number(document.getElementById('defaultPollIntervalSec').value)
        };
        const res = await fetch('/api/config', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        if (!res.ok) {
          alert('Failed to save config');
          return;
        }
        await loadConfig();
      } finally {
        button.disabled = false;
      }
    }

    async function triggerScan() {
      const button = document.getElementById('scanBtn');
      button.disabled = true;
      try {
        const res = await fetch('/api/scan', { method: 'POST' });
        if (!res.ok) {
          alert('Failed to trigger scan');
        }
      } finally {
        button.disabled = false;
      }
    }

    async function refreshAll() {
      await loadConfig();
      await loadDevices();
      await loadTrends();
    }

    refreshAll();
    setInterval(async () => {
      await loadDevices();
      await loadTrends();
    }, 5000);
  </script>
</body>
</html>`

func (s *webServer) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprint(w, indexHTML)
}
