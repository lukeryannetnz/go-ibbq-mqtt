package main

import (
	"testing"
)

func TestFloatToStringSingle(t *testing.T) {
	data := []float64{6}
	expected := "[6.000000]"
	var got = f64tostring(data)

	if got != expected {
		t.Errorf("getTopic('hello') = %s; want %s", got, expected)
	}
}

func TestFloatToString(t *testing.T) {
	data := []float64{6, 15.1456}
	expected := "[6.000000 15.145600]"
	var got = f64tostring(data)

	if got != expected {
		t.Errorf("getTopic('hello') = %s; want %s", got, expected)
	}
}
