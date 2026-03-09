package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Update struct {
	TS    int64  `json:"ts"`
	Topic string `json:"topic"`
	Value int64  `json:"value"`
	Raw   string `json:"raw"`
}

type MQTTSubscriber struct {
	cfg    MQTTConfig
	db     *DB
	client mqtt.Client
	broker *Broker
}

func NewMQTTSubscriber(cfg MQTTConfig, db *DB, broker *Broker) *MQTTSubscriber {
	return &MQTTSubscriber{cfg: cfg, db: db, broker: broker}
}

func (m *MQTTSubscriber) Start() error {
	addr := fmt.Sprintf("tcp://%s:%d", m.cfg.Host, m.cfg.Port)
	host, _ := os.Hostname()
	opts := mqtt.NewClientOptions().
		AddBroker(addr).
		SetClientID("r3p-dashboard-" + host).
		SetAutoReconnect(true).
		SetOnConnectHandler(m.onConnect).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Printf("MQTT connection lost: %v", err)
		}).
		SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
			log.Println("MQTT reconnecting...")
		})

	m.client = mqtt.NewClient(opts)
	token := m.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	log.Printf("Connected to MQTT broker at %s", addr)
	return nil
}

func (m *MQTTSubscriber) onConnect(c mqtt.Client) {
	topic := m.cfg.TopicPrefix + "/#"
	token := c.Subscribe(topic, 0, m.handleMessage)
	token.Wait()
	if err := token.Error(); err != nil {
		log.Printf("MQTT subscribe error: %v", err)
		return
	}
	log.Printf("Subscribed to %s", topic)
}

func (m *MQTTSubscriber) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	fullTopic := msg.Topic()
	short := strings.TrimPrefix(fullTopic, m.cfg.TopicPrefix+"/")
	payload := string(msg.Payload())
	ts := time.Now().Unix()

	if short == "fault" {
		if err := m.db.InsertEvent(ts, "fault", payload); err != nil {
			log.Printf("Insert event error: %v", err)
		}
		m.broker.Publish(Update{TS: ts, Topic: "fault", Value: 0, Raw: payload})
		return
	}

	reg, ok := TopicRegistry[short]
	if !ok {
		return
	}

	value, err := parseValue(payload, reg.Scale)
	if err != nil {
		log.Printf("Parse error for %s=%q: %v", short, payload, err)
		return
	}

	if err := m.db.InsertReading(ts, short, value); err != nil {
		log.Printf("Insert reading error: %v", err)
		return
	}

	m.broker.Publish(Update{TS: ts, Topic: short, Value: value, Raw: payload})
}

func (m *MQTTSubscriber) Stop() {
	if m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(500)
	}
}

func parseValue(payload string, scale int) (int64, error) {
	switch payload {
	case "true":
		return 1, nil
	case "false":
		return 0, nil
	}

	if scale > 1 {
		f, err := strconv.ParseFloat(payload, 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(f * float64(scale))), nil
	}

	i, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(payload, 64)
		if ferr != nil {
			return 0, err
		}
		return int64(math.Round(f)), nil
	}
	return i, nil
}
