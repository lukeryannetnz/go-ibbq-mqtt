package main

import (
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MqttClient is the public interface
type MqttClient interface {
	Init()
	Pub(topic string, payload interface{})
}

// mqttClient implements the MqttClient interface, encapsulating the paho.mqtt.golang module.
type mqttClient struct {
	client mqtt.Client
}

func NewMqttClient() MqttClient {
	c := &mqttClient{}

	return c
}

func (m *mqttClient) Init() {
	opts := mqtt.NewClientOptions().AddBroker(os.Getenv("MQTT_SERVER")).SetClientID("go-ibbq-mqtt")
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	m.client = mqtt.NewClient(opts)

	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		logger.Fatal("Error connecting to mqtt", "err", token.Error())
	}

	logger.Info("Connecting to mqtt broker", "broker", os.Getenv("MQTT_SERVER"))
}

func (m *mqttClient) Pub(topic string, payload interface{}) {
	statustoken := m.client.Publish(getTopic(topic), 0, false, payload)
	statustoken.Wait()
}

func getTopic(topic string) string {
	return fmt.Sprint("ibbq/", topic)
}
