package main

import "encoding/json"

type batteryLevel struct {
	BatteryLevel int
}

func (b *batteryLevel) toJson() string {
	j, _ := json.Marshal(b)

	return string(j)
}
