package main

import(
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MqttClient is the public interface
type MqttClient interface{
	Init()
	Pub(payload interface{})
}

// mqttClient implements the MqttClient interface, encapsulating the paho.mqtt.golang module.
type mqttClient struct{
	client mqtt.Client
}

func NewMqttClient() MqttClient {
	c := &mqttClient{}

	return c;
}

func (m *mqttClient) Init(){
	opts := mqtt.NewClientOptions().AddBroker("tcp://iot.eclipse.org:1883").SetClientID("go-ibbq-mqtt")
	opts.SetKeepAlive(2 * time.Second)
	//opts.SetDefaultPublishHandler(f)
	opts.SetPingTimeout(1 * time.Second)

	m.client = mqtt.NewClient(opts)

	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		logger.Fatal("Error connecting to mqtt", "err", token.Error())
	}

	logger.Info("Connecting to mqtt broker", "broker", "tcp://iot.eclipse.org:1883")
}

func (m *mqttClient) Pub(payload interface{}){
	statustoken := m.client.Publish("ibbq/data", 0, false, payload)
	statustoken.Wait()
}