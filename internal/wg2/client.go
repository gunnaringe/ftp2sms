package wg2

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"log/slog"
	"sync"
	"time"
)

type Sms struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Content string `json:"content"`
}

type Payload struct {
	Sms *Sms `json:"sms"`
}

type Client struct {
	logger       *slog.Logger
	mqttClient   mqtt.Client
	smsCallbacks []CallbackFunc
	smsMutex     sync.Mutex
}

type CallbackFunc func(sms Sms)

func NewClient(logger *slog.Logger, mqttServer string) *Client {
	c := &Client{
		logger: logger,
	}
	mqttClient := c.connect(mqttServer)
	c.mqttClient = mqttClient
	return c
}

func (c *Client) SendSms(sms Sms) error {
	c.logger.Info("Send SMS", "from", sms.From, "to", sms.To)
	return publish(c, "wg2/outbox/sms", Payload{Sms: &sms})
}

func (c *Client) OnSms(callback CallbackFunc) {
	c.smsMutex.Lock()
	c.smsCallbacks = append(c.smsCallbacks, callback)
	c.smsMutex.Unlock()
}

func (c *Client) connect(broker string) mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("ftp2sms").
		SetConnectionLostHandler(c.connectionLostHandler).
		SetOnConnectHandler(c.onConnectHandler)

	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		c.logger.Error("Connection error", "error", token.Error())
	}
	return mqttClient
}

func publish(c *Client, topic string, payload interface{}) error {
	msg, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	token := c.mqttClient.Publish(topic, 1, false, msg)
	token.Wait()
	if token.Error() != nil {
		log.Fatalf("Failed to publish message: %s", token.Error())
		return token.Error()
	}
	return nil
}

func (c *Client) onConnectHandler(client mqtt.Client) {
	fmt.Println("Connected")
	if token := client.Subscribe("wg2/inbox/#", 1, c.messageHandler); token.Wait() && token.Error() != nil {
		log.Fatalf("Subscription error: %s", token.Error())
	}
}

func (c *Client) connectionLostHandler(client mqtt.Client, err error) {
	fmt.Printf("Connection lost: %s\n", err)
	backOffTime := 1 * time.Second
	for {
		if token := client.Connect(); token.Wait() && token.Error() == nil {
			fmt.Println("Reconnected")
			break
		}
		fmt.Printf("Reconnection attempt failed. Retrying in %s...\n", backOffTime)
		time.Sleep(backOffTime)
		if backOffTime < 60*time.Second {
			backOffTime *= 2
		}
	}
}

func (c *Client) messageHandler(client mqtt.Client, msg mqtt.Message) {
	var payload Payload
	err := json.Unmarshal(msg.Payload(), &payload)
	if err != nil {
		c.logger.Warn("Failed to unmarshal payload", "error", err)
	}

	if msg.Topic() == "wg2/inbox/sms" {
		sms := payload.Sms
		if sms == nil {
			c.logger.Warn("Empty payload for SMS")
			return
		}
		c.smsMutex.Lock()
		for _, callback := range c.smsCallbacks {
			callback(*sms)
		}
		c.smsMutex.Unlock()
	}
}
