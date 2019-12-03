package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.gazette.dev/core/broker/client"
	pb "go.gazette.dev/core/broker/protocol"
	"go.gazette.dev/core/brokertest"
	pc "go.gazette.dev/core/consumer/protocol"
	"go.gazette.dev/core/consumertest"
	"go.gazette.dev/core/etcdtest"
	"go.gazette.dev/core/labels"
	"go.gazette.dev/core/mainboilerplate/runconsumer"
	"go.gazette.dev/core/message"
)

func TestPingPong(t *testing.T) {
	var etcd = etcdtest.TestClient()
	defer etcdtest.Cleanup()

	var testJournals, testShards = buildSpecFixtures(4)

	// Start a broker & create journal fixtures.
	var broker = brokertest.NewBroker(t, etcd, "local", "broker")
	var rjc = pb.NewRoutedJournalClient(broker.Client(), pb.NoopDispatchRouter{})
	brokertest.CreateJournals(t, broker, testJournals...)

	var app = new(App)
	var cfg = app.NewConfig()
	cfg.(*config).PingPong.Players = 100
	cfg.(*config).PingPong.Period = 0

	// Start and serve a consumer, and create shard fixtures.
	var cmr = consumertest.NewConsumer(consumertest.Args{
		C:        t,
		Etcd:     etcd,
		Journals: rjc,
		App:      app,
	})
	cmr.Tasks.GoRun()

	assert.NoError(t, app.InitApplication(
		runconsumer.InitArgs{
			Context: context.Background(),
			Config:  cfg,
			Server:  cmr.Server,
			Service: cmr.Service,
		}))

	consumertest.CreateShards(t, cmr, testShards...)

	// Start one ping-pong game.
	var as = client.NewAppendService(context.Background(), broker.Client())
	startOneGame(app.mapping, message.NewPublisher(as, nil), *cfg.(*config))

	// Read one of the partitions, and expect to see an ongoing stream of volleys.
	var it = message.NewReadCommittedIter(
		client.NewRetryReader(context.Background(), broker.Client(),
			pb.ReadRequest{Journal: testJournals[0].Name, Block: true},
		),
		func(spec *pb.JournalSpec) (i message.Message, e error) {
			return new(Volley), nil
		},
		message.NewSequencer(nil, 512),
	)
	for i := 0; i != 10; i++ {
		var env, err = it.Next()
		assert.NoError(t, err)
		_ = env.Message.(*Volley)
		t.Log(env.Message.(*Volley))
	}

	// Shutdown.
	cmr.Tasks.Cancel()
	assert.NoError(t, cmr.Tasks.Wait())

	broker.Tasks.Cancel()
	assert.NoError(t, broker.Tasks.Wait())
}

func buildSpecFixtures(parts int) (journals []*pb.JournalSpec, shards []*pc.ShardSpec) {
	for p := 0; p != parts; p++ {
		var (
			part  = fmt.Sprintf("%02d", p)
			shard = &pc.ShardSpec{
				Id: pc.ShardID("part-" + part),
				Sources: []pc.ShardSpec_Source{
					{Journal: pb.Journal("volleys/part=" + part)},
				},
				RecoveryLogPrefix: "recovery/logs",
				HintPrefix:        "/gazette/hints",
				MaxTxnDuration:    time.Second,
			}
		)
		journals = append(journals,
			brokertest.Journal(pb.JournalSpec{
				Name: shard.Sources[0].Journal,
				LabelSet: pb.MustLabelSet(
					labels.MessageType, "ping_pong.Volley",
					labels.ContentType, labels.ContentType_JSONLines,
				),
			}),
			brokertest.Journal(pb.JournalSpec{
				Name:     shard.RecoveryLog(),
				LabelSet: pb.MustLabelSet(labels.ContentType, labels.ContentType_RecoveryLog),
			}),
		)
		shards = append(shards, shard)
	}
	return
}
