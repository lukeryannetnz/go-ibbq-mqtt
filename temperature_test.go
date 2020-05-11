package main

import (
	"testing"
)

func TestTemperatureJsonSingle(t *testing.T) {
	data := &temperature{[]float64{6}}
	expected := "{\"Temperatures\":[6]}" // {"Temperatures":[6]}
	var got = data.toJson()

	if got != expected {
		t.Errorf("got %s; want %s", got, expected)
	}
}

func TestTestTemperatureJsonMany(t *testing.T) {
	data := &temperature{[]float64{6, 15.1456}}
	expected := "{\"Temperatures\":[6,15.1456]}" // {"Temperatures":[6,15.1456]}
	var got = data.toJson()

	if got != expected {
		t.Errorf("got %s; want %s", got, expected)
	}
}
