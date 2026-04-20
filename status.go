package main

import (
	"encoding/json"
	"time"
)

type status struct {
	Status    string
	Timestamp string
}

func (s *status) toJson() string {
	j, _ := json.Marshal(s)

	return string(j)
}

func publishStatus(deviceName, st string) {
	s := &status{
		Status:    st,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	mc.Pub(deviceName, "status", s.toJson())
}
