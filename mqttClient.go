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
	Pub(deviceName, topic string, payload interface{})
}

// mqttClient implements the MqttClient interface, encapsulating the paho.mqtt.golang module.
type mqttClient struct {
	client   mqtt.Client
	registry *Registry
}

func NewMqttClient(registry *Registry) MqttClient {
	c := &mqttClient{registry: registry}

	return c
}

func (m *mqttClient) Init() {
	logger.Info("Connecting to mqtt broker", "broker", os.Getenv("MQTT_SERVER"))

	opts := mqtt.NewClientOptions().AddBroker(os.Getenv("MQTT_SERVER")).SetClientID("go-ibbq-mqtt")
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	m.client = mqtt.NewClient(opts)

	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		if m.registry != nil {
			m.registry.SetMQTTConnected(false)
		}
		logger.Fatal("Error connecting to mqtt", "err", token.Error())
	}

	if m.registry != nil {
		m.registry.SetMQTTConnected(true)
	}
	logger.Info("Connected to mqtt broker", "broker", os.Getenv("MQTT_SERVER"))
}

func (m *mqttClient) Pub(deviceName, topic string, payload interface{}) {
	t := getTopic(deviceName, topic)

	logger.Info("Publishing to mqtt", "topic", t)
	statustoken := m.client.Publish(t, 0, false, payload)

	statustoken.Wait()
	if statustoken.Error() != nil {
		if m.registry != nil {
			m.registry.SetMQTTConnected(false)
		}
		logger.Error("Error publishing to mqtt", "err", statustoken.Error())
		return
	}

	if m.registry != nil {
		m.registry.SetMQTTConnected(true)
		m.registry.RecordMQTTPublish(t, payload)
	}
}

func getTopic(deviceName, topic string) string {
	base := os.Getenv("MQTT_TOPIC")
	if deviceName == "" || deviceName == "default" {
		return fmt.Sprintf("%s/%s", base, topic)
	}

	return fmt.Sprintf("%s/%s/%s", base, deviceName, topic)
}
