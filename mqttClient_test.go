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
	configureenv()
}

func TestGetTopic(t *testing.T) {
	var got = getTopic("hello")
	if got != "ibbq/hello" {
		t.Errorf("getTopic('hello') = %s; want ibbq/hello", got)
	}
}
