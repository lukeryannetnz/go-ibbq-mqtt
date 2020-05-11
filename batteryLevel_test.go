package main

import (
	"testing"
)

func TestBatteryLevelString(t *testing.T) {
	data := &batteryLevel{69}
	expected := "{\"BatteryLevel\":69}" // {"BatteryLevel":69}
	var got = data.toJson()

	if got != expected {
		t.Errorf("got %s; want %s", got, expected)
	}
}
