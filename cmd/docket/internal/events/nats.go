package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("docket/events/nats")

// natsBus publishes events to NATS JetStream (persistent, replayable) so
// downstream consumers can subscribe and process them at their own pace.
// A goroutine also subscribes and logs whatever comes back — real forks
// would replace that logger with actual work (thumbnailer, indexer, etc).
type natsBus struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	subject   string   // e.g. "docket.files"
	stream    string   // e.g. "DOCKET_EVENTS"
	consumer  jetstream.Consumer
	consumeCC jetstream.ConsumeContext
	log       *slog.Logger
}

func newNATS(ctx context.Context, cfg config.NATSConfig, log *slog.Logger) (*natsBus, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	nc, err := nats.Connect(cfg.URL,
		nats.Timeout(2*time.Second),
		nats.RetryOnFailedConnect(false),
	)
	if err != nil {
		return nil, err
	}
	// Ensure the connection actually completed (nats.Connect is otherwise fast-fail-friendly).
	if !nc.IsConnected() {
		nc.Close()
		return nil, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}

	// Create-or-update the stream that captures every event under the subject prefix.
	// Wildcards: "docket.files.>" catches docket.files.uploaded, docket.files.deleted, etc.
	streamName := cfg.Stream
	if streamName == "" {
		streamName = "DOCKET_EVENTS"
	}
	subjectPrefix := cfg.SubjectPrefix
	if subjectPrefix == "" {
		subjectPrefix = "docket.files"
	}

	_, err = js.CreateOrUpdateStream(dialCtx, jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subjectPrefix + ".>"},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    24 * time.Hour, // POC: keep a day of events
	})
	if err != nil {
		nc.Close()
		return nil, err
	}

	// Set up a consumer that just logs every message it sees, matching the
	// pattern the previous event-bus code used.
	consumer, err := js.CreateOrUpdateConsumer(dialCtx, streamName, jetstream.ConsumerConfig{
		Name:          "docket-log-consumer",
		Durable:       "docket-log-consumer",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: subjectPrefix + ".>",
	})
	if err != nil {
		nc.Close()
		return nil, err
	}

	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		log.Info("nats event consumed",
			"subject", msg.Subject(),
			"bytes", len(msg.Data()),
		)
		_ = msg.Ack()
	})
	if err != nil {
		nc.Close()
		return nil, err
	}

	log.Info("nats event bus connected",
		"url", cfg.URL,
		"stream", streamName,
		"subject_prefix", subjectPrefix,
	)
	return &natsBus{
		nc:        nc,
		js:        js,
		subject:   subjectPrefix,
		stream:    streamName,
		consumer:  consumer,
		consumeCC: cc,
		log:       log,
	}, nil
}

// subjectFor turns an event type ("file.uploaded") into a NATS subject
// ("docket.files.uploaded"). It strips the leading "file." from the type so
// the hierarchical subject reads cleanly.
func (n *natsBus) subjectFor(evType string) string {
	// event.Type is like "file.uploaded" — take the second word and hang it
	// under the subject prefix. Fall back to the raw type if there's no dot.
	if i := indexDot(evType); i >= 0 {
		return n.subject + "." + evType[i+1:]
	}
	return n.subject + "." + evType
}

func indexDot(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func (n *natsBus) Publish(ctx context.Context, ev Event) error {
	subject := n.subjectFor(ev.Type)

	ctx, span := tracer.Start(ctx, "nats.Publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination", subject),
		attribute.String("messaging.nats.stream", n.stream),
		attribute.String("event.type", ev.Type),
		attribute.String("event.file_id", ev.FileID),
	)

	body, err := ev.Marshal()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	_, err = n.js.Publish(ctx, subject, body)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (n *natsBus) Mode() string { return "live" }

func (n *natsBus) Close(_ context.Context) error {
	if n.consumeCC != nil {
		n.consumeCC.Stop()
	}
	n.nc.Close()
	return nil
}
