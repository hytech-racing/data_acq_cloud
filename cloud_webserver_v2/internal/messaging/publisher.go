package messaging

import (
	"context"
	"sync"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

type SubscribedMessage struct {
	content *utils.DecodedMessage
	ctx     context.Context
}

func (sm *SubscribedMessage) GetContent() *utils.DecodedMessage {
	return sm.content
}

type Publisher struct {
	subscribers  map[string]chan SubscribedMessage
	results_chan chan SubscriberResult
	end_results  map[string]SubscriberResult
	mutex        sync.Mutex
	wg           sync.WaitGroup
	resultsWg    sync.WaitGroup
}

type SubscriberResult struct {
	SubscriberID   int
	SubscriberName string
	ResultData     map[string]interface{}
}

func NewPublisher(enableResultsListener bool) *Publisher {
	var results_chan chan SubscriberResult = nil
	if enableResultsListener {
		results_chan = make(chan SubscriberResult)
	}

	publisher := &Publisher{
		subscribers:  make(map[string]chan SubscribedMessage),
		results_chan: results_chan,
		end_results:  make(map[string]SubscriberResult),
	}

	if enableResultsListener {
		publisher.initCollectResults()
	}

	return publisher
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
		subFunc(id, subscriberName, channel, p.results_chan)
	}()
}

// Publishes a new message to all subscribers in subscriberNames
func (p *Publisher) Publish(ctx context.Context, message *utils.DecodedMessage, subscriberNames []string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	subscriberMessage := SubscribedMessage{
		content: message,
		ctx:     ctx,
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

	if p.results_chan != nil {
		close(p.results_chan)
		p.resultsWg.Wait()
	}
}

func (p *Publisher) initCollectResults() {
	p.resultsWg.Add(1)

	go func() {
		defer p.resultsWg.Done()
		p.collectResults(p.results_chan)
	}()
}

func (p *Publisher) collectResults(results_chan <-chan SubscriberResult) {
	for msg := range results_chan {
		p.mutex.Lock()
		p.end_results[msg.SubscriberName] = msg
		p.mutex.Unlock()
	}
}

func (p *Publisher) GetResults() map[string]SubscriberResult {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.end_results
}
