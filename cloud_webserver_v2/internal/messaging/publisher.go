package messaging

import (
	"sync"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

type SubscribedMessage struct {
	content *utils.DecodedMessage
}

func (sm *SubscribedMessage) GetContent() *utils.DecodedMessage {
	return sm.content
}

type Publisher struct {
	subscribers map[string]chan SubscribedMessage
	mutex       sync.Mutex
	wg          sync.WaitGroup
}

func NewPublisher() *Publisher {
	return &Publisher{
		subscribers: make(map[string]chan SubscribedMessage),
	}
}

// Subscribe adds a new subscriber channel to the publisher
func (p *Publisher) Subscribe(id int, subscriberName string, subFunc SubscriberFunc) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	channel := make(chan SubscribedMessage)
	p.subscribers[subscriberName] = channel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		subFunc(id, channel)
	}()
}

// Publishes a new message to all subscribers in subscriberNames
func (p *Publisher) Publish(message *utils.DecodedMessage, subscriberNames []string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	subscriberMessage := SubscribedMessage{
		content: message,
	}

	for _, sub := range subscriberNames {
		if ch, ok := p.subscribers[sub]; ok {
			ch <- subscriberMessage
		}
	}
}

// Closes all subscriber channels
func (p *Publisher) CloseAllSubscribers() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, ch := range p.subscribers {
		close(ch)
	}
}

func (p *Publisher) WaitForClosure() {
	p.wg.Wait()
}
