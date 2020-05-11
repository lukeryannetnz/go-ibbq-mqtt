package main

import "testing"

func TestTestStatusJson(t *testing.T) {
	data := &status{"working!!"}
	expected := "{\"Status\":\"working!!\"}" // {"Status":"working!!"}
	var got = data.toJson()

	if got != expected {
		t.Errorf("got %s; want %s", got, expected)
	}
}
