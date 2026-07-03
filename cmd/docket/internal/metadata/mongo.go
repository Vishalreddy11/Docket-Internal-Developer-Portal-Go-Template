package metadata

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

const collection = "files"

type mongoStore struct {
	client *mongo.Client
	coll   *mongo.Collection
}

func newMongo(ctx context.Context, cfg config.MongoConfig, log *slog.Logger) (*mongoStore, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	opts := options.Client().ApplyURI(cfg.URI).SetMonitor(otelmongo.NewMonitor())
	client, err := mongo.Connect(dialCtx, opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(dialCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	log.Info("mongo metadata store connected", "uri", cfg.URI, "db", cfg.DB)
	return &mongoStore{
		client: client,
		coll:   client.Database(cfg.DB).Collection(collection),
	}, nil
}

func (m *mongoStore) Insert(ctx context.Context, meta Meta) error {
	_, err := m.coll.InsertOne(ctx, meta)
	return err
}

func (m *mongoStore) Get(ctx context.Context, id string) (Meta, error) {
	var meta Meta
	err := m.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&meta)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Meta{}, ErrNotFound
	}
	return meta, err
}

func (m *mongoStore) List(ctx context.Context, limit, offset int) ([]Meta, error) {
	opts := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset)).SetSort(bson.D{{Key: "uploaded_at", Value: -1}})
	cur, err := m.coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Meta
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *mongoStore) Delete(ctx context.Context, id string) error {
	_, err := m.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (m *mongoStore) Mode() string { return "live" }
func (m *mongoStore) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

var ErrNotFound = errors.New("metadata: not found")
