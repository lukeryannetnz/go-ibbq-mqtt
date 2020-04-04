package main

import (
	"testing"
)

func TestGetTopic(t *testing.T) {
	var got = getTopic("hello")
	if got != "ibbq/hello" {
		t.Errorf("getTopic('hello') = %s; want ibbq/hello", got)
	}
}
