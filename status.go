package main

import "encoding/json"

type status struct {
	Status string
}

func (s *status) toJson() string {
	j, _ := json.Marshal(s)

	return string(j)
}
