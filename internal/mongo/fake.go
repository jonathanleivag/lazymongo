package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// FakeClient is an in-memory Client for unit-testing TUI logic without a
// real database. Data is organized as FakeClient.Databases[db][coll] = docs.
type FakeClient struct {
	Databases map[string]map[string][]bson.M
	Indexes   map[string]map[string][]IndexInfo

	ConnectErr error
	nextID     int
}

var _ Client = (*FakeClient)(nil)

func NewFakeClient() *FakeClient {
	return &FakeClient{
		Databases: map[string]map[string][]bson.M{},
		Indexes:   map[string]map[string][]IndexInfo{},
	}
}

func (f *FakeClient) Connect(ctx context.Context, uri string) error { return f.ConnectErr }
func (f *FakeClient) Disconnect(ctx context.Context) error          { return nil }

func (f *FakeClient) ListDatabases(ctx context.Context) ([]string, error) {
	var names []string
	for name := range f.Databases {
		names = append(names, name)
	}
	return names, nil
}

func (f *FakeClient) ListCollections(ctx context.Context, db string) ([]string, error) {
	var names []string
	for name := range f.Databases[db] {
		names = append(names, name)
	}
	return names, nil
}

func (f *FakeClient) Find(ctx context.Context, db, coll string, filter bson.M, sortDoc bson.M, skip, limit int64) ([]bson.M, error) {
	docs := f.Databases[db][coll]
	if int64(len(docs)) <= skip {
		return []bson.M{}, nil
	}
	end := skip + limit
	if end > int64(len(docs)) || limit == 0 {
		end = int64(len(docs))
	}
	return docs[skip:end], nil
}

func (f *FakeClient) CountDocuments(ctx context.Context, db, coll string, filter bson.M) (int64, error) {
	return int64(len(f.Databases[db][coll])), nil
}

func (f *FakeClient) InsertOne(ctx context.Context, db, coll string, doc bson.M) (any, error) {
	f.nextID++
	id := fmt.Sprintf("fake-id-%d", f.nextID)
	doc["_id"] = id
	if f.Databases[db] == nil {
		f.Databases[db] = map[string][]bson.M{}
	}
	f.Databases[db][coll] = append(f.Databases[db][coll], doc)
	return id, nil
}

func (f *FakeClient) UpdateField(ctx context.Context, db, coll string, id any, field string, value any) error {
	for _, doc := range f.Databases[db][coll] {
		if doc["_id"] == id {
			doc[field] = value
			return nil
		}
	}
	return fmt.Errorf("documento %v no encontrado", id)
}

func (f *FakeClient) ReplaceOne(ctx context.Context, db, coll string, id any, doc bson.M) error {
	docs := f.Databases[db][coll]
	for i, d := range docs {
		if d["_id"] == id {
			doc["_id"] = id
			docs[i] = doc
			return nil
		}
	}
	return fmt.Errorf("documento %v no encontrado", id)
}

func (f *FakeClient) DeleteOne(ctx context.Context, db, coll string, id any) error {
	docs := f.Databases[db][coll]
	for i, d := range docs {
		if d["_id"] == id {
			f.Databases[db][coll] = append(docs[:i], docs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("documento %v no encontrado", id)
}

func (f *FakeClient) ListIndexes(ctx context.Context, db, coll string) ([]IndexInfo, error) {
	return f.Indexes[db][coll], nil
}

func (f *FakeClient) CreateIndex(ctx context.Context, db, coll string, keys bson.D, unique bool) (string, error) {
	name := ""
	keyMap := bson.M{}
	for _, e := range keys {
		if name != "" {
			name += "_"
		}
		name += fmt.Sprintf("%s_%v", e.Key, e.Value)
		keyMap[e.Key] = e.Value
	}
	if f.Indexes[db] == nil {
		f.Indexes[db] = map[string][]IndexInfo{}
	}
	f.Indexes[db][coll] = append(f.Indexes[db][coll], IndexInfo{Name: name, Key: keyMap, Unique: unique})
	return name, nil
}

func (f *FakeClient) DropIndex(ctx context.Context, db, coll, name string) error {
	idxs := f.Indexes[db][coll]
	for i, idx := range idxs {
		if idx.Name == name {
			f.Indexes[db][coll] = append(idxs[:i], idxs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("índice %q no encontrado", name)
}

func (f *FakeClient) CreateCollection(ctx context.Context, db, coll string) error {
	if f.Databases[db] == nil {
		f.Databases[db] = map[string][]bson.M{}
	}
	if _, exists := f.Databases[db][coll]; exists {
		return fmt.Errorf("la collection %q ya existe en %q", coll, db)
	}
	f.Databases[db][coll] = []bson.M{}
	return nil
}

func (f *FakeClient) DropCollection(ctx context.Context, db, coll string) error {
	if _, exists := f.Databases[db][coll]; !exists {
		return fmt.Errorf("la collection %q no existe en %q", coll, db)
	}
	delete(f.Databases[db], coll)
	delete(f.Indexes[db], coll)
	return nil
}

func (f *FakeClient) DropDatabase(ctx context.Context, db string) error {
	delete(f.Databases, db)
	delete(f.Indexes, db)
	return nil
}

func (f *FakeClient) RenameCollection(ctx context.Context, db, oldName, newName string) error {
	docs, exists := f.Databases[db][oldName]
	if !exists {
		return fmt.Errorf("la collection %q no existe en %q", oldName, db)
	}
	if oldName != newName {
		if _, collides := f.Databases[db][newName]; collides {
			return fmt.Errorf("ya existe una collection llamada %q en %q", newName, db)
		}
	}
	delete(f.Databases[db], oldName)
	f.Databases[db][newName] = docs
	if idxs, ok := f.Indexes[db][oldName]; ok {
		delete(f.Indexes[db], oldName)
		f.Indexes[db][newName] = idxs
	}
	return nil
}

func (f *FakeClient) RunAdminCommand(ctx context.Context, cmd bson.D) (bson.M, error) {
	if len(cmd) > 0 {
		switch cmd[0].Key {
		case "serverStatus":
			return bson.M{
				"mem": bson.M{
					"virtual":  int32(64830),
					"resident": int32(44450),
				},
				"connections": bson.M{
					"current":   int32(2366),
					"available": int32(10000),
				},
				"network": bson.M{
					"bytesIn":  int64(602000),
					"bytesOut": int64(2427000),
				},
				"opcounters": bson.M{
					"insert":  int64(2),
					"query":   int64(0),
					"update":  int64(54),
					"delete":  int64(0),
					"getmore": int64(344),
					"command": int64(341),
				},
			}, nil
		case "currentOp":
			return bson.M{
				"inprog": bson.A{
					bson.M{
						"opid":              int64(482910),
						"ns":                "haddacloud-v2.email_sents",
						"microsecs_running": int64(804090),
						"op":                "getmore",
					},
					bson.M{
						"opid":              int64(482911),
						"ns":                "chatbot-flows.flows",
						"microsecs_running": int64(716720),
						"op":                "getmore",
					},
				},
			}, nil
		}
	}
	return bson.M{}, nil
}
