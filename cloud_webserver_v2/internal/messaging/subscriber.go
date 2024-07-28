package messaging

import "fmt"

// Subscriber function type
type SubscriberFunc func(id int, ch <-chan SubscribedMessage)

func PrintMessages(id int, ch <-chan SubscribedMessage) {
	for msg := range ch {
		fmt.Printf("content: %v \n", msg.GetContent().Data)
	}
}
