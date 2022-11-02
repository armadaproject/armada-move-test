package eventstream

import "github.com/armadaproject/armada/pkg/api"

type AckFn func() error

type Message struct {
	EventMessage *api.EventMessage
	Ack          AckFn
}

type EventStream interface {
	Publish(events []*api.EventMessage) []error
	Subscribe(queue string, callback func(event *Message) error) error
	Close() error
}
