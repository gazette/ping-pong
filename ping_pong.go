package main

import (
	"time"

	"github.com/pkg/errors"
	"go.gazette.dev/core/broker/client"
	pb "go.gazette.dev/core/broker/protocol"
	"go.gazette.dev/core/consumer"
	"go.gazette.dev/core/consumer/recoverylog"
	"go.gazette.dev/core/labels"
	"go.gazette.dev/core/mainboilerplate/runconsumer"
	"go.gazette.dev/core/message"
)

// config configures the ping-pong application.
type config struct {
	runconsumer.BaseConfig

	// Fizzle demonstrates how application-specific parameters can be configured
	// by runconsumer.Main, as either process flags or from the environment.
	Fizzle struct {
		Flopp int    `long:"flopp" default:"42" description:"Flippity flopp"`
		Blarg string `long:"blarg" default:"klarble" description:"Blargle"`
	} `group:"Fizzle" namespace:"fizzle" env-namespace:"FIZZLE"`
}

const (
	// FirstServeLabel indicates we're responsible for the first game serve.
	FirstServeLabel = "first-serve"
	// VolleyToLabel indicates to whom return volleys are directed.
	VolleyToLabel = "volley-to"
)

// Volley is a Message representing a volley between game participants.
type Volley struct {
	UUID     message.UUID
	From, To string
	Round    int
}

// Implementation of the message.Message interface.
func (v *Volley) SetUUID(uuid message.UUID)                     { v.UUID = uuid }
func (v *Volley) GetUUID() message.UUID                         { return v.UUID }
func (v *Volley) NewAcknowledgement(pb.Journal) message.Message { return new(Volley) }

// App implements our runconsumer.Application.
type App struct {
	mapping message.MappingFunc
	pub     *message.Publisher
}

// state which is represented by each shard's consumer.Store.
type state struct {
	ReceivedVolleys int
}

func (p *App) NewStore(shard consumer.Shard, rec *recoverylog.Recorder) (store consumer.Store, err error) {
	var state = new(state)

	if store, err = consumer.NewJSONFileStore(rec, state); err != nil {
		return nil, err
	}

	// Is this the first round, and we're responsible for first serve?
	if shard.Spec().LabelSet.ValuesOf(FirstServeLabel) != nil && state.ReceivedVolleys == 0 {
		var id = shard.Spec().Id.String()

		if _, err = p.pub.PublishCommitted(p.mapping, &Volley{
			From: id,
			To:   id,
		}); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (p *App) NewMessage(*pb.JournalSpec) (message.Message, error) {
	return new(Volley), nil
}

func (p *App) ConsumeMessage(shard consumer.Shard, store consumer.Store, env message.Envelope, pub *message.Publisher) error {
	var (
		volley = env.Message.(*Volley)
		id     = shard.Spec().Id.String()
		state  = store.(*consumer.JSONFileStore).State.(*state)
	)

	if volley.To != id {
		return nil // Not our ball.
	}
	state.ReceivedVolleys++

	var _, err = pub.PublishUncommitted(p.mapping, &Volley{
		From:  id,
		To:    shard.Spec().LabelSet.ValueOf(VolleyToLabel),
		Round: state.ReceivedVolleys,
	})
	return err
}

func (p *App) FinalizeTxn(consumer.Shard, consumer.Store, *message.Publisher) error {
	return nil // No-op.
}

func (p *App) NewConfig() runconsumer.Config { return new(config) }

func (p *App) InitApplication(args runconsumer.InitArgs) error {
	if args.Config.(*config).Fizzle.Flopp != 42 {
		return errors.New("expected 'flopp' to be 42")
	}

	// Select all journals having message-type=ping_pong.Volley.
	var partitions, err = client.NewPolledList(args.Context, args.Service.Journals, 30*time.Second,
		pb.ListRequest{
			Selector: pb.LabelSelector{
				Include: pb.MustLabelSet(labels.MessageType, "ping_pong.Volley"),
			},
		})
	if err != nil {
		return err
	}
	p.mapping = message.RandomMapping(partitions.List)

	return nil
}

func main() { runconsumer.Main(new(App)) }
