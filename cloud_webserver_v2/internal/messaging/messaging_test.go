package messaging

import (
	"testing"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

type testSubscriber struct {
	testing    *testing.T
	output     string
	subscriber string
}

func dummySubscriber(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	for msg := range ch {
		if msg.GetContent().Topic == "done" {
			if results != nil {
				results <- SubscriberResult{
					SubscriberID:   id,
					SubscriberName: subscriberName,
					ResultData:     map[string]interface{}{"output": "finished"},
				}
			}
			return
		}
	}
}

func TestPublisherSubscribeAndPublish(t *testing.T) {
	pub := NewPublisher(true)

	pub.Subscribe(1, "worker1", dummySubscriber)

	msg1 := &utils.DecodedMessage{Topic: "random", Data: map[string]interface{}{}, LogTime: 0}
	msg2 := &utils.DecodedMessage{Topic: "done", Data: map[string]interface{}{}, LogTime: 0}

	pub.Publish(msg1, []string{"worker1"})
	pub.Publish(msg2, []string{"worker1"})

	pub.CloseAllSubscribers()
	pub.WaitForClosure()

	results := pub.Results()
	res, ok := results["worker1"]
	if !ok {
		t.Fatalf("Expected result for 'worker1' not found")
	}

	if output, exists := res.ResultData["output"]; !exists || output != "finished" {
		t.Errorf("Expected output 'finished', got %v", output)
	}
}

func TestPublisherNoResults(t *testing.T) {
	pub := NewPublisher(false)
	pub.Subscribe(2, "worker2", dummySubscriber)

	msg := &utils.DecodedMessage{Topic: "done", Data: map[string]interface{}{}, LogTime: 0}
	pub.Publish(msg, []string{"worker2"})

	pub.CloseAllSubscribers()
	pub.WaitForClosure()

	results := pub.Results()
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}
