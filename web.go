package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type deviceState struct {
	mu           sync.RWMutex `json:"-"`
	Name         string
	Temperatures []float64
	BatteryLevel int
	Status       string
	LastSeen     time.Time
}

func (s *deviceState) snapshot() deviceState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	temperatures := append([]float64(nil), s.Temperatures...)
	return deviceState{
		Name:         s.Name,
		Temperatures: temperatures,
		BatteryLevel: s.BatteryLevel,
		Status:       s.Status,
		LastSeen:     s.LastSeen,
	}
}

var (
	deviceStates   = make(map[string]*deviceState)
	deviceStatesMu sync.RWMutex
)

func getOrCreateDeviceState(name string) *deviceState {
	deviceStatesMu.Lock()
	defer deviceStatesMu.Unlock()

	if s, ok := deviceStates[name]; ok {
		return s
	}

	s := &deviceState{Name: name}
	deviceStates[name] = s
	return s
}

func startWebServer(port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices", handleDeviceList)
	mux.HandleFunc("/api/devices/", handleDevice)
	mux.HandleFunc("/", handleIndex)

	addr := fmt.Sprintf(":%s", port)
	logger.Info("Starting web server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("Web server error", "err", err)
	}
}

func handleDeviceList(w http.ResponseWriter, _ *http.Request) {
	deviceStatesMu.RLock()
	states := make([]deviceState, 0, len(deviceStates))
	for _, s := range deviceStates {
		states = append(states, s.snapshot())
	}
	deviceStatesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(states)
}

func handleDevice(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/devices/")

	deviceStatesMu.RLock()
	s, ok := deviceStates[name]
	deviceStatesMu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.snapshot())
}

var indexHTML = "<!DOCTYPE html>\n" +
	"<html>\n" +
	"<head>\n" +
	"<meta charset=\"utf-8\">\n" +
	"<meta http-equiv=\"refresh\" content=\"5\">\n" +
	"<title>iBBQ Monitor</title>\n" +
	"<style>\n" +
	"body { font-family: monospace; padding: 1em; }\n" +
	".device { border: 1px solid #ccc; padding: 1em; margin: 0.5em 0; display: inline-block; min-width: 200px; vertical-align: top; }\n" +
	".label { color: #666; }\n" +
	"</style>\n" +
	"</head>\n" +
	"<body>\n" +
	"<h2>iBBQ Monitor</h2>\n" +
	"<div id=\"devices\"></div>\n" +
	"<script>\n" +
	"fetch('/api/devices').then(r=>r.json()).then(devices=>{\n" +
	"    const el = document.getElementById('devices');\n" +
	"    if (!devices || devices.length === 0) { el.textContent = 'No devices'; return; }\n" +
	"    el.innerHTML = devices.map(d => {\n" +
	"        const temps = (d.Temperatures || []).map((t, i) => '<span class=\"label\">T' + (i + 1) + ':</span> ' + Number(t).toFixed(1) + '°C').join('<br>');\n" +
	"        return '<div class=\"device\">' +\n" +
	"            '<strong>' + (d.Name || '--') + '</strong><br>' +\n" +
	"            '<span class=\"label\">Status:</span> ' + (d.Status || '--') + '<br>' +\n" +
	"            '<span class=\"label\">Battery:</span> ' + (d.BatteryLevel != null ? d.BatteryLevel + '%' : '--') + '<br>' +\n" +
	"            temps + '<br>' +\n" +
	"            '<span class=\"label\">Last seen:</span> ' + (d.LastSeen ? new Date(d.LastSeen).toLocaleTimeString() : '--') +\n" +
	"        '</div>';\n" +
	"    }).join('');\n" +
	"});\n" +
	"</script>\n" +
	"</body>\n" +
	"</html>"

func handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprint(w, indexHTML)
}
