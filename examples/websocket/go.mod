module github.com/lukeryannetnz/go-ibbq/v2/examples/websocket

go 1.12

require (
	github.com/containous/flaeg v1.4.1
	github.com/containous/staert v3.1.2+incompatible
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/gin-gonic/gin v1.6.2
	github.com/go-ble/ble v0.0.0-20181002102605-e78417b510a3
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/consul v1.4.0 // indirect
	github.com/kisielk/gotool v1.0.0 // indirect
	github.com/mgutz/logxi v0.0.0-20161027140823-aebf8a7d67ab
	github.com/sworisbreathing/go-ibbq/v2 v2.0.0
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e // indirect
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
	gopkg.in/go-playground/validator.v8 v8.18.2 // indirect
)

replace github.com/sworisbreathing/go-ibbq/v2 v2.0.0 => ../../
