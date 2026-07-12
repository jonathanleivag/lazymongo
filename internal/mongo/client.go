package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// IndexInfo describes one index on a collection.
type IndexInfo struct {
	Name   string
	Key    bson.M
	Unique bool
}

// Client is every MongoDB operation lazymongo's TUI needs. It exists so TUI
// logic can be unit-tested against FakeClient instead of a real database.
type Client interface {
	Connect(ctx context.Context, uri string) error
	Disconnect(ctx context.Context) error

	ListDatabases(ctx context.Context) ([]string, error)
	ListCollections(ctx context.Context, db string) ([]string, error)

	Find(ctx context.Context, db, coll string, filter bson.M, skip, limit int64) ([]bson.M, error)
	CountDocuments(ctx context.Context, db, coll string, filter bson.M) (int64, error)
	InsertOne(ctx context.Context, db, coll string, doc bson.M) (any, error)
	UpdateField(ctx context.Context, db, coll string, id any, field string, value any) error
	ReplaceOne(ctx context.Context, db, coll string, id any, doc bson.M) error
	DeleteOne(ctx context.Context, db, coll string, id any) error

	ListIndexes(ctx context.Context, db, coll string) ([]IndexInfo, error)
	CreateIndex(ctx context.Context, db, coll string, keys bson.D, unique bool) (string, error)
	DropIndex(ctx context.Context, db, coll, name string) error
}

// RealClient implements Client against the official MongoDB Go driver.
type RealClient struct {
	client *driver.Client
}

func NewRealClient() *RealClient {
	return &RealClient{}
}

func (c *RealClient) Connect(ctx context.Context, uri string) error {
	client, err := driver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("conectando a mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("ping a mongo falló: %w", err)
	}
	c.client = client
	return nil
}

func (c *RealClient) Disconnect(ctx context.Context) error {
	if c.client == nil {
		return nil
	}
	return c.client.Disconnect(ctx)
}

func (c *RealClient) ListDatabases(ctx context.Context) ([]string, error) {
	names, err := c.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("listando bases de datos: %w", err)
	}
	return names, nil
}

func (c *RealClient) ListCollections(ctx context.Context, db string) ([]string, error) {
	cursor, err := c.client.Database(db).ListCollections(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("listando colecciones de %s: %w", db, err)
	}
	defer cursor.Close(ctx)

	var names []string
	for cursor.Next(ctx) {
		var info bson.M
		if err := cursor.Decode(&info); err != nil {
			return nil, fmt.Errorf("decodificando colección: %w", err)
		}
		if name, ok := info["name"].(string); ok {
			names = append(names, name)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterando colecciones de %s: %w", db, err)
	}
	return names, nil
}

func (c *RealClient) Find(ctx context.Context, db, coll string, filter bson.M, skip, limit int64) ([]bson.M, error) {
	opts := options.Find().SetSkip(skip)
	if limit > 0 {
		opts.SetLimit(limit)
	}
	cursor, err := c.client.Database(db).Collection(coll).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("buscando documentos en %s.%s: %w", db, coll, err)
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("leyendo resultados de %s.%s: %w", db, coll, err)
	}
	return results, nil
}

func (c *RealClient) CountDocuments(ctx context.Context, db, coll string, filter bson.M) (int64, error) {
	count, err := c.client.Database(db).Collection(coll).CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("contando documentos en %s.%s: %w", db, coll, err)
	}
	return count, nil
}

func (c *RealClient) InsertOne(ctx context.Context, db, coll string, doc bson.M) (any, error) {
	result, err := c.client.Database(db).Collection(coll).InsertOne(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("insertando documento en %s.%s: %w", db, coll, err)
	}
	return result.InsertedID, nil
}

func (c *RealClient) UpdateField(ctx context.Context, db, coll string, id any, field string, value any) error {
	filter := bson.M{"_id": id}
	update := bson.M{"$set": bson.M{field: value}}
	result, err := c.client.Database(db).Collection(coll).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("actualizando campo %q en %s.%s: %w", field, db, coll, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("documento %v no encontrado en %s.%s", id, db, coll)
	}
	return nil
}

func (c *RealClient) ReplaceOne(ctx context.Context, db, coll string, id any, doc bson.M) error {
	filter := bson.M{"_id": id}
	result, err := c.client.Database(db).Collection(coll).ReplaceOne(ctx, filter, doc)
	if err != nil {
		return fmt.Errorf("reemplazando documento en %s.%s: %w", db, coll, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("documento %v no encontrado en %s.%s", id, db, coll)
	}
	return nil
}

func (c *RealClient) DeleteOne(ctx context.Context, db, coll string, id any) error {
	filter := bson.M{"_id": id}
	result, err := c.client.Database(db).Collection(coll).DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("borrando documento en %s.%s: %w", db, coll, err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("documento %v no encontrado en %s.%s", id, db, coll)
	}
	return nil
}
