package messaging

import (
	"sync"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

/*
This file just serves as a way for messages (right now specifically MCAP messages) to be send to a bunch of workers working asynchronously.
After those async workers complete, they can send a result back. Publisher doesn't do anything with those. It just collects them.
Performing operations on those results is up to the code using the publisher.
*/

type SubscribedMessage struct {
	content *utils.DecodedMessage
}

func (sm *SubscribedMessage) GetContent() *utils.DecodedMessage {
	return sm.content
}

type SubscriberResults map[string]SubscriberResult

type Publisher struct {
	subscribers  map[string]chan SubscribedMessage
	results_chan chan SubscriberResult
	end_results  SubscriberResults
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
		end_results:  make(SubscriberResults),
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

func (p *Publisher) initCollectResults() {
	p.resultsWg.Add(1)

	go func() {
		defer p.resultsWg.Done()
		p.collectResults(p.results_chan)
	}()
}

// Closes all subscriber channels
func (p *Publisher) CloseAllSubscribers() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, ch := range p.subscribers {
		close(ch)
	}
}

// Waits for all the subscribers to close and closes the results channel
func (p *Publisher) WaitForClosure() {
	p.wg.Wait()

	// We don't close the results channel in CloseAllSubscribers because the subscribers return results when closed. We need to wait for those results to come in.
	if p.results_chan != nil {
		close(p.results_chan)
		p.resultsWg.Wait()
	}
}

func (p *Publisher) collectResults(results_chan <-chan SubscriberResult) {
	for msg := range results_chan {
		p.mutex.Lock()
		p.end_results[msg.SubscriberName] = msg
		p.mutex.Unlock()
	}
}

func (p *Publisher) Results() SubscriberResults {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.end_results
}
