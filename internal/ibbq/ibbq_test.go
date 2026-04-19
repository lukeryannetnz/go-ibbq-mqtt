package ibbq

import (
	"testing"
	"time"
)

func TestRealTimeDataReceivedFiltersNoProbeSentinel(t *testing.T) {
	gotCh := make(chan []float64, 1)
	thermometer := Ibbq{
		temperatureReceivedHandler: func(values []float64) {
			gotCh <- values
		},
	}

	handler := thermometer.realTimeDataReceived()
	handler([]byte{0xE2, 0x00, 0xF6, 0xFF, 0xEA, 0x00})

	select {
	case got := <-gotCh:
		if len(got) != 2 || got[0] != 22.6 || got[1] != 23.4 {
			t.Fatalf("got %v, want [22.6 23.4]", got)
		}
	case <-time.After(time.Second):
		t.Fatal("temperatureReceivedHandler was not called")
	}
}

func TestRealTimeDataReceivedSkipsAllEmptyProbePayloads(t *testing.T) {
	gotCh := make(chan []float64, 1)
	thermometer := Ibbq{
		temperatureReceivedHandler: func(values []float64) {
			gotCh <- values
		},
	}

	handler := thermometer.realTimeDataReceived()
	handler([]byte{0xF6, 0xFF, 0xF6, 0xFF})

	select {
	case got := <-gotCh:
		t.Fatalf("unexpected callback with %v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
