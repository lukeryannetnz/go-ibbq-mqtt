package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	os.Exit(code)
}

func setup() {
	configureEnv()
	_ = os.Setenv("MQTT_TOPIC", "ibbq")
}

func TestGetTopic(t *testing.T) {
	var got = getTopic("default", "hello")
	if got != "ibbq/hello" {
		t.Errorf("getTopic('default', 'hello') = %s; want ibbq/hello", got)
	}
}

func TestGetTopicWithDeviceName(t *testing.T) {
	var got = getTopic("probe1", "hello")
	if got != "ibbq/probe1/hello" {
		t.Errorf("getTopic('probe1', 'hello') = %s; want ibbq/probe1/hello", got)
	}
}
