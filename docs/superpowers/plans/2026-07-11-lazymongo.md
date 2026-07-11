# lazymongo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a personal, keyboard-driven TUI for browsing and editing MongoDB (databases → collections → documents → indexes), reusing the existing `~/.config/mongo-connections.sh` connection file.

**Architecture:** Go + Bubbletea (Elm architecture) + Lipgloss for styling, official `go.mongodb.org/mongo-driver/v2` for MongoDB access. A `mongo.Client` interface abstracts the driver so TUI logic is unit-testable with a fake. Connection resolution shells out to `zsh` (not `bash` — macOS's system `bash` is 3.2 and predates associative arrays) to source the real `mongo-connections.sh`, so there is exactly one source of truth shared with the existing `mgo` shell function.

**Tech Stack:** Go 1.23+, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `go.mongodb.org/mongo-driver/v2`.

## Global Constraints

- Module path: `github.com/jonathanleivag/lazymongo`
- Connection file: `~/.config/mongo-connections.sh` (zsh — sourced by `.zshrc`; uses `declare -A` associative arrays, which macOS's system `bash` 3.2 cannot parse, so all tooling that reads/writes it must invoke `zsh`, never `bash`), existing format extended with a `MONGO_CONNECTION_COLORS` array — never touched except via the tested insertion logic in Task 3
- Every mutating action (insert, inline edit, full-document replace, delete, create index, drop index) MUST show a `y`/`n` confirmation before executing, on every connection, no exceptions
- Colors are exactly one of: `amarillo`, `rojo`, `verde`, or empty string (no color assigned yet)
- No aggregation pipeline builder, no schema viewer, no connection edit/delete UI — out of scope for this plan (see spec's "Future ideas")
- Integration tests that talk to a real MongoDB must NEVER run against `qa`/`prod` — only a disposable local `docker run --rm mongo:7`, and only under the `integration` build tag so plain `go test ./...` never needs Docker

---

### Task 1: Project scaffolding

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `.gitignore`
- Create: `README.md`

**Interfaces:**
- Consumes: nothing (first task)
- Produces: a running `go build` and a `lazymongo` binary that prints a placeholder message and exits — later tasks replace the placeholder body of `main.go`

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
cd ~/Development/jonathanleivag/lazymongo
go mod init github.com/jonathanleivag/lazymongo
```
Expected: creates `go.mod` with `module github.com/jonathanleivag/lazymongo` and a `go` directive.

- [ ] **Step 2: Add a placeholder main.go**

```go
package main

import "fmt"

func main() {
	fmt.Println("lazymongo: not implemented yet")
}
```

- [ ] **Step 3: Add .gitignore**

```
/lazymongo
*.test
```

- [ ] **Step 4: Add a minimal README**

```markdown
# lazymongo

Personal TUI for browsing/editing MongoDB. See `docs/superpowers/specs/2026-07-11-lazymongo-design.md`
for the design, and `docs/superpowers/plans/2026-07-11-lazymongo.md` for the implementation plan.

## Build

    go build -o lazymongo .

## Run

    ./lazymongo <connection-name>   # resolves from ~/.config/mongo-connections.sh
    ./lazymongo                     # shows a picker of available connections
```

- [ ] **Step 5: Verify it builds and runs**

Run: `go build -o lazymongo . && ./lazymongo`
Expected: prints `lazymongo: not implemented yet`

- [ ] **Step 6: Commit**

```bash
git add go.mod main.go .gitignore README.md
git commit -m "chore: scaffold lazymongo Go module"
```

---

### Task 2: Read connections from mongo-connections.sh

**Files:**
- Create: `internal/config/connections.go`
- Test: `internal/config/connections_test.go`
- Create: `internal/config/testdata/basic.sh`
- Create: `internal/config/testdata/no_colors.sh`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type config.Connection struct { Name, URI, Color string }`
  - `func config.ResolveConnection(name string) (config.Connection, error)`
  - `func config.ListConnections() ([]config.Connection, error)`
  - package var `config.connectionsFile string` (overridable by tests)

- [ ] **Step 1: Add fixture files**

`internal/config/testdata/basic.sh`:
```bash
declare -A MONGO_CONNECTIONS=(
  [qa]="mongodb://localhost:27017/test"
  [prod]="mongodb://localhost:27017/prod"
)

declare -A MONGO_CONNECTION_COLORS=(
  [qa]="verde"
  [prod]="rojo"
)
```

`internal/config/testdata/no_colors.sh`:
```bash
declare -A MONGO_CONNECTIONS=(
  [solo]="mongodb://localhost:27017/solo"
)
```

- [ ] **Step 2: Write the failing tests**

```go
package config

import (
	"sort"
	"testing"
)

func withConnectionsFile(t *testing.T, path string) {
	t.Helper()
	old := connectionsFile
	connectionsFile = path
	t.Cleanup(func() { connectionsFile = old })
}

func TestResolveConnection_Found(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Connection{Name: "qa", URI: "mongodb://localhost:27017/test", Color: "verde"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestResolveConnection_NoColorAssigned(t *testing.T) {
	withConnectionsFile(t, "testdata/no_colors.sh")

	conn, err := ResolveConnection("solo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Connection{Name: "solo", URI: "mongodb://localhost:27017/solo", Color: ""}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestResolveConnection_NotFound(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	_, err := ResolveConnection("nope")
	if err == nil {
		t.Fatal("expected an error for an unknown connection name, got nil")
	}
}

func TestListConnections(t *testing.T) {
	withConnectionsFile(t, "testdata/basic.sh")

	conns, err := ListConnections()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Slice(conns, func(i, j int) bool { return conns[i].Name < conns[j].Name })

	if len(conns) != 2 {
		t.Fatalf("got %d connections, want 2: %+v", len(conns), conns)
	}
	if conns[0].Name != "prod" || conns[1].Name != "qa" {
		t.Fatalf("got names %q, %q; want prod, qa", conns[0].Name, conns[1].Name)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd ~/Development/jonathanleivag/lazymongo && go test ./internal/config/... -v`
Expected: FAIL — `connectionsFile`, `Connection`, `ResolveConnection`, `ListConnections` undefined

- [ ] **Step 3: Implement connections.go**

```go
package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Connection is one named MongoDB connection from ~/.config/mongo-connections.sh.
type Connection struct {
	Name  string
	URI   string
	Color string // "amarillo" | "rojo" | "verde" | ""
}

var connectionsFile = defaultConnectionsFile()

func defaultConnectionsFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mongo-connections.sh")
}

// ResolveConnection reads the URI and color for a single named connection by
// shelling out to zsh and sourcing the real connections file, so this stays
// in lockstep with the `mgo` shell function.
func ResolveConnection(name string) (Connection, error) {
	script := fmt.Sprintf(
		`source %q; echo "${MONGO_CONNECTIONS[%s]}"; echo "${MONGO_CONNECTION_COLORS[%s]}"`,
		connectionsFile, name, name,
	)
	out, err := runShell(script)
	if err != nil {
		return Connection{}, fmt.Errorf("resolviendo conexión %q: %w", name, err)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var uri, color string
	if len(lines) > 0 {
		uri = lines[0]
	}
	if len(lines) > 1 {
		color = lines[1]
	}
	if uri == "" {
		return Connection{}, fmt.Errorf("no existe la conexión %q en %s", name, connectionsFile)
	}
	return Connection{Name: name, URI: uri, Color: color}, nil
}

// ListConnections returns every named connection, sorted by name.
func ListConnections() ([]Connection, error) {
	script := fmt.Sprintf(`source %q; printf "%%s\n" "${(@k)MONGO_CONNECTIONS}"`, connectionsFile)
	out, err := runShell(script)
	if err != nil {
		return nil, fmt.Errorf("listando conexiones: %w", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return []Connection{}, nil
	}

	names := strings.Split(trimmed, "\n")
	conns := make([]Connection, 0, len(names))
	for _, name := range names {
		conn, err := ResolveConnection(name)
		if err != nil {
			continue
		}
		conns = append(conns, conn)
	}
	sort.Slice(conns, func(i, j int) bool { return conns[i].Name < conns[j].Name })
	return conns, nil
}

// runShell sources the connections file via zsh — NOT bash. macOS ships
// bash 3.2 (Apple never updates it past the last GPLv2 release), which
// predates associative arrays (`declare -A`, added in bash 4.0) and fails
// with "declare: -A: invalid option". zsh has supported associative arrays
// since 4.0 and is the guaranteed-present, up-to-date default shell on
// macOS — it's also what the real `mgo` shell function runs under, so using
// it here keeps behavior identical to sourcing the file from `.zshrc`.
func runShell(script string) (string, error) {
	cmd := exec.Command("zsh", "-c", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return out.String(), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/config/connections.go internal/config/connections_test.go internal/config/testdata
git commit -m "feat: read named connections from mongo-connections.sh"
```

---

### Task 3: Write new connections into mongo-connections.sh

**Files:**
- Create: `internal/config/writer.go`
- Test: `internal/config/writer_test.go`
- Create: `internal/config/testdata/with_comments.sh`

**Interfaces:**
- Consumes: `Connection` (Task 2), `connectionsFile` var (Task 2)
- Produces: `func config.AddConnection(conn Connection) error`

- [ ] **Step 1: Add a fixture with comments (mirrors the real file's header)**

`internal/config/testdata/with_comments.sh`:
```bash
# Conexiones de MongoDB nombradas, usadas por la función `mgo` de .zshrc.
# Este archivo NO se sube a ningún repo — solo vive en esta Mac.
# Formato: [nombre]="uri completa de mongodb"

declare -A MONGO_CONNECTIONS=(
  [ejemplo-local]="mongodb://localhost:27017"
)
```
(Note: this fixture intentionally has no `MONGO_CONNECTION_COLORS` block yet, matching the owner's real file at the time this plan was written.)

- [ ] **Step 2: Write the failing tests**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempConnectionsFile copies a fixture to a temp file and points
// connectionsFile at the copy, so tests never mutate testdata.
func withTempConnectionsFile(t *testing.T, fixture string) string {
	t.Helper()
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "mongo-connections.sh")
	if err := os.WriteFile(tmp, src, 0600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	old := connectionsFile
	connectionsFile = tmp
	t.Cleanup(func() { connectionsFile = old })
	return tmp
}

func TestAddConnection_ToFileWithColorsBlock(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	err := AddConnection(Connection{Name: "staging", URI: "mongodb://10.0.0.9:27017/db", Color: "amarillo"})
	if err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	conn, err := ResolveConnection("staging")
	if err != nil {
		t.Fatalf("resolving new connection: %v", err)
	}
	want := Connection{Name: "staging", URI: "mongodb://10.0.0.9:27017/db", Color: "amarillo"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}

	// existing connections must still resolve correctly
	if _, err := ResolveConnection("qa"); err != nil {
		t.Fatalf("existing connection broke after AddConnection: %v", err)
	}
}

func TestAddConnection_CreatesMissingColorsBlock(t *testing.T) {
	withTempConnectionsFile(t, "testdata/with_comments.sh")

	err := AddConnection(Connection{Name: "nueva", URI: "mongodb://localhost:27017/nueva", Color: "rojo"})
	if err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	conn, err := ResolveConnection("nueva")
	if err != nil {
		t.Fatalf("resolving new connection: %v", err)
	}
	if conn.Color != "rojo" {
		t.Fatalf("got color %q, want rojo", conn.Color)
	}

	// the pre-existing connection (with no color) must be untouched
	old, err := ResolveConnection("ejemplo-local")
	if err != nil {
		t.Fatalf("resolving pre-existing connection: %v", err)
	}
	if old.URI != "mongodb://localhost:27017" || old.Color != "" {
		t.Fatalf("pre-existing connection changed unexpectedly: %+v", old)
	}
}

func TestAddConnection_HyphenatedName(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	err := AddConnection(Connection{Name: "movatec-prod", URI: "mongodb://x:27017/y", Color: "verde"})
	if err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}
	if _, err := ResolveConnection("movatec-prod"); err != nil {
		t.Fatalf("resolving hyphenated connection: %v", err)
	}
}

func TestAddConnection_ResultIsValidZsh(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := AddConnection(Connection{Name: "x", URI: "mongodb://x", Color: "verde"}); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/config/... -v -run TestAddConnection`
Expected: FAIL — `AddConnection`, `validateZshSyntax` undefined

- [ ] **Step 4: Implement writer.go**

```go
package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AddConnection appends a new named connection (and its color) to the real
// connections file, validating the result is still valid zsh before
// keeping the change. On any failure, the original file content is restored.
func AddConnection(conn Connection) error {
	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}

	updated, err := insertConnection(string(original), conn)
	if err != nil {
		return err
	}

	if err := os.WriteFile(connectionsFile, []byte(updated), 0600); err != nil {
		return fmt.Errorf("escribiendo %s: %w", connectionsFile, err)
	}

	if err := validateZshSyntax(connectionsFile); err != nil {
		_ = os.WriteFile(connectionsFile, original, 0600)
		return fmt.Errorf("el archivo resultante no era zsh válido, se revirtió: %w", err)
	}
	return nil
}

func insertConnection(content string, conn Connection) (string, error) {
	content, err := insertIntoArray(content, "MONGO_CONNECTIONS", fmt.Sprintf("  [%s]=%q", conn.Name, conn.URI))
	if err != nil {
		return "", err
	}
	content, err = insertIntoArray(content, "MONGO_CONNECTION_COLORS", fmt.Sprintf("  [%s]=%q", conn.Name, conn.Color))
	if err != nil {
		return "", err
	}
	return content, nil
}

// insertIntoArray inserts newLine just before the closing ")" of the named
// zsh associative array, creating the array block at the end of the file
// if it doesn't exist yet.
func insertIntoArray(content, arrayName, newLine string) (string, error) {
	header := fmt.Sprintf("declare -A %s=(", arrayName)

	if !strings.Contains(content, header) {
		block := fmt.Sprintf("\ndeclare -A %s=(\n%s\n)\n", arrayName, newLine)
		return content + block, nil
	}

	lines := strings.Split(content, "\n")
	headerLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			headerLineIdx = i
			break
		}
	}
	if headerLineIdx == -1 {
		return "", fmt.Errorf("se encontró %q pero no en su propia línea, no se pudo editar %s de forma segura", header, arrayName)
	}

	for i := headerLineIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == ")" {
			result := make([]string, 0, len(lines)+1)
			result = append(result, lines[:i]...)
			result = append(result, newLine)
			result = append(result, lines[i:]...)
			return strings.Join(result, "\n"), nil
		}
	}
	return "", fmt.Errorf("no se encontró el cierre ')' del array %s", arrayName)
}

// validateZshSyntax uses zsh (not bash — see runShell) in no-exec mode to
// check the file parses without actually running it.
func validateZshSyntax(path string) error {
	cmd := exec.Command("zsh", "-n", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS (all tests from Task 2 and Task 3)

- [ ] **Step 6: Commit**

```bash
git add internal/config/writer.go internal/config/writer_test.go internal/config/testdata/with_comments.sh
git commit -m "feat: add AddConnection to safely append new connections"
```

---

### Task 4: MongoClient interface + connect/list operations

**Files:**
- Create: `internal/mongo/client.go`
- Create: `internal/mongo/fake.go`
- Test: `internal/mongo/client_integration_test.go`
- Create: `scripts/test-integration.sh`

**Interfaces:**
- Consumes: nothing (new package)
- Produces:
  - `type mongo.IndexInfo struct { Name string; Key bson.M; Unique bool }`
  - `type mongo.Client interface` with methods `Connect`, `Disconnect`, `ListDatabases`, `ListCollections`, `Find`, `CountDocuments`, `InsertOne`, `UpdateField`, `ReplaceOne`, `DeleteOne`, `ListIndexes`, `CreateIndex`, `DropIndex` (full signatures below — later tasks implement the document/index methods' bodies, but the interface and struct are fixed here)
  - `type mongo.RealClient struct` implementing `Client` against the real driver (`Connect`/`Disconnect`/`ListDatabases`/`ListCollections` implemented now; the rest implemented in Tasks 5–6)
  - `type mongo.FakeClient struct` implementing `Client` in-memory, for TUI unit tests in later tasks

- [ ] **Step 1: Add the MongoDB Go driver dependency**

Run:
```bash
cd ~/Development/jonathanleivag/lazymongo
go get go.mongodb.org/mongo-driver/v2/mongo@latest
```
Expected: `go.mod`/`go.sum` updated with `go.mongodb.org/mongo-driver/v2`.

- [ ] **Step 2: Write client.go with the full interface and the Connect/Disconnect/List* implementations**

```go
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
```

- [ ] **Step 3: Write fake.go (in-memory fake used by all internal/tui tests)**

```go
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

func (f *FakeClient) Find(ctx context.Context, db, coll string, filter bson.M, skip, limit int64) ([]bson.M, error) {
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
```

- [ ] **Step 4: Add the integration test (build-tagged, requires Docker)**

```go
//go:build integration

package mongo

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRealClient_ConnectAndListDatabases(t *testing.T) {
	uri := os.Getenv("LAZYMONGO_TEST_URI")
	if uri == "" {
		t.Skip("LAZYMONGO_TEST_URI not set; run via scripts/test-integration.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewRealClient()
	if err := client.Connect(ctx, uri); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect(ctx)

	names, err := client.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDatabases failed: %v", err)
	}
	found := false
	for _, n := range names {
		if n == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'admin' database to be present, got %v", names)
	}
}
```

- [ ] **Step 5: Add the disposable-Mongo test harness script**

`scripts/test-integration.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

CONTAINER=lazymongo-test-mongo
docker run --rm -d --name "$CONTAINER" -p 27018:27017 mongo:7 >/dev/null

cleanup() {
  docker stop "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "Esperando a que MongoDB esté listo..."
for _ in $(seq 1 30); do
  if docker exec "$CONTAINER" mongosh --quiet --eval 'db.runCommand({ping:1})' >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

LAZYMONGO_TEST_URI="mongodb://localhost:27018" go test -tags=integration ./... -v
```

- [ ] **Step 6: Make the script executable and verify it builds (Docker not required for this step)**

Run:
```bash
chmod +x scripts/test-integration.sh
go build ./...
go vet ./...
```
Expected: no errors. (The integration test itself is skipped unless `LAZYMONGO_TEST_URI` is set — do not run `scripts/test-integration.sh` yet unless Docker Desktop is running.)

- [ ] **Step 7: Commit**

```bash
git add internal/mongo/client.go internal/mongo/fake.go internal/mongo/client_integration_test.go scripts/test-integration.sh go.mod go.sum
git commit -m "feat: add MongoClient interface, real+fake implementations, connect/list ops"
```

---

### Task 5: Document operations (Find, Count, Insert, UpdateField, ReplaceOne, DeleteOne)

**Files:**
- Modify: `internal/mongo/client.go`
- Modify: `internal/mongo/client_integration_test.go`

**Interfaces:**
- Consumes: `Client` interface and `RealClient` struct (Task 4)
- Produces: working method bodies for `Find`, `CountDocuments`, `InsertOne`, `UpdateField`, `ReplaceOne`, `DeleteOne` on `RealClient`

- [ ] **Step 1: Add integration tests for document operations**

Append to `internal/mongo/client_integration_test.go`:

```go
func TestRealClient_DocumentCRUD(t *testing.T) {
	uri := os.Getenv("LAZYMONGO_TEST_URI")
	if uri == "" {
		t.Skip("LAZYMONGO_TEST_URI not set; run via scripts/test-integration.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewRealClient()
	if err := client.Connect(ctx, uri); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect(ctx)

	const db, coll = "lazymongo_test", "widgets"

	id, err := client.InsertOne(ctx, db, coll, bson.M{"name": "gizmo", "qty": 3})
	if err != nil {
		t.Fatalf("InsertOne failed: %v", err)
	}

	docs, err := client.Find(ctx, db, coll, bson.M{"name": "gizmo"}, 0, 10)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	count, err := client.CountDocuments(ctx, db, coll, bson.M{})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	if err := client.UpdateField(ctx, db, coll, id, "qty", 5); err != nil {
		t.Fatalf("UpdateField failed: %v", err)
	}
	docs, _ = client.Find(ctx, db, coll, bson.M{"_id": id}, 0, 1)
	if len(docs) != 1 || docs[0]["qty"] != int32(5) {
		t.Fatalf("expected qty updated to 5, got %+v", docs)
	}

	if err := client.ReplaceOne(ctx, db, coll, id, bson.M{"name": "gizmo-v2"}); err != nil {
		t.Fatalf("ReplaceOne failed: %v", err)
	}
	docs, _ = client.Find(ctx, db, coll, bson.M{"_id": id}, 0, 1)
	if len(docs) != 1 || docs[0]["name"] != "gizmo-v2" {
		t.Fatalf("expected replaced document, got %+v", docs)
	}

	if err := client.DeleteOne(ctx, db, coll, id); err != nil {
		t.Fatalf("DeleteOne failed: %v", err)
	}
	count, _ = client.CountDocuments(ctx, db, coll, bson.M{})
	if count != 0 {
		t.Fatalf("expected count 0 after delete, got %d", count)
	}
}
```

- [ ] **Step 2: Verify the new test fails to compile (methods missing bodies / return errors)**

Run: `go vet ./... 2>&1 | head -30`
Expected: compiles (methods already declared on the interface from Task 4), but running with Docker would fail since bodies aren't implemented yet — implement them now.

- [ ] **Step 3: Implement the document operation methods**

Append to `internal/mongo/client.go`:

```go
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
```

- [ ] **Step 4: Run the integration test against a disposable Mongo**

Run (requires Docker Desktop running):
```bash
./scripts/test-integration.sh
```
Expected: `TestRealClient_ConnectAndListDatabases` and `TestRealClient_DocumentCRUD` both PASS.

- [ ] **Step 5: Run the plain unit test suite too (no Docker needed)**

Run: `go test ./...`
Expected: PASS (integration tests skip themselves without `LAZYMONGO_TEST_URI`)

- [ ] **Step 6: Commit**

```bash
git add internal/mongo/client.go internal/mongo/client_integration_test.go
git commit -m "feat: implement document CRUD operations on RealClient"
```

---

### Task 6: Index operations (ListIndexes, CreateIndex, DropIndex)

**Files:**
- Modify: `internal/mongo/client.go`
- Modify: `internal/mongo/client_integration_test.go`

**Interfaces:**
- Consumes: `Client` interface, `IndexInfo` struct (Task 4)
- Produces: working method bodies for `ListIndexes`, `CreateIndex`, `DropIndex` on `RealClient`

- [ ] **Step 1: Add the integration test**

Append to `internal/mongo/client_integration_test.go`:

```go
func TestRealClient_Indexes(t *testing.T) {
	uri := os.Getenv("LAZYMONGO_TEST_URI")
	if uri == "" {
		t.Skip("LAZYMONGO_TEST_URI not set; run via scripts/test-integration.sh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewRealClient()
	if err := client.Connect(ctx, uri); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect(ctx)

	const db, coll = "lazymongo_test", "indexed_widgets"
	_, _ = client.InsertOne(ctx, db, coll, bson.M{"sku": "abc"})

	name, err := client.CreateIndex(ctx, db, coll, bson.D{{Key: "sku", Value: 1}}, true)
	if err != nil {
		t.Fatalf("CreateIndex failed: %v", err)
	}
	if name == "" {
		t.Fatal("expected a non-empty index name")
	}

	indexes, err := client.ListIndexes(ctx, db, coll)
	if err != nil {
		t.Fatalf("ListIndexes failed: %v", err)
	}
	found := false
	for _, idx := range indexes {
		if idx.Name == name {
			found = true
			if !idx.Unique {
				t.Fatalf("expected index %q to be unique", name)
			}
		}
	}
	if !found {
		t.Fatalf("created index %q not found in ListIndexes result: %+v", name, indexes)
	}

	if err := client.DropIndex(ctx, db, coll, name); err != nil {
		t.Fatalf("DropIndex failed: %v", err)
	}
	indexes, _ = client.ListIndexes(ctx, db, coll)
	for _, idx := range indexes {
		if idx.Name == name {
			t.Fatalf("index %q still present after DropIndex", name)
		}
	}
}
```

- [ ] **Step 2: Implement the index operation methods**

Append to `internal/mongo/client.go`:

```go
func (c *RealClient) ListIndexes(ctx context.Context, db, coll string) ([]IndexInfo, error) {
	cursor, err := c.client.Database(db).Collection(coll).Indexes().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando índices de %s.%s: %w", db, coll, err)
	}
	defer cursor.Close(ctx)

	var infos []IndexInfo
	for cursor.Next(ctx) {
		var raw struct {
			Name   string `bson:"name"`
			Key    bson.M `bson:"key"`
			Unique bool   `bson:"unique"`
		}
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decodificando índice de %s.%s: %w", db, coll, err)
		}
		infos = append(infos, IndexInfo{Name: raw.Name, Key: raw.Key, Unique: raw.Unique})
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterando índices de %s.%s: %w", db, coll, err)
	}
	return infos, nil
}

func (c *RealClient) CreateIndex(ctx context.Context, db, coll string, keys bson.D, unique bool) (string, error) {
	model := driver.IndexModel{
		Keys:    keys,
		Options: options.Index().SetUnique(unique),
	}
	name, err := c.client.Database(db).Collection(coll).Indexes().CreateOne(ctx, model)
	if err != nil {
		return "", fmt.Errorf("creando índice en %s.%s: %w", db, coll, err)
	}
	return name, nil
}

func (c *RealClient) DropIndex(ctx context.Context, db, coll, name string) error {
	if err := c.client.Database(db).Collection(coll).Indexes().DropOne(ctx, name); err != nil {
		return fmt.Errorf("borrando índice %q en %s.%s: %w", name, db, coll, err)
	}
	return nil
}
```

- [ ] **Step 3: Run the integration suite**

Run: `./scripts/test-integration.sh`
Expected: all `TestRealClient_*` tests PASS, including `TestRealClient_Indexes`.

- [ ] **Step 4: Run the plain unit test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mongo/client.go internal/mongo/client_integration_test.go
git commit -m "feat: implement index operations on RealClient"
```

---

### Task 7: Reusable list component and confirmation modal

**Files:**
- Create: `internal/tui/list.go`
- Test: `internal/tui/list_test.go`
- Create: `internal/tui/confirm.go`
- Test: `internal/tui/confirm_test.go`
- Create: `internal/tui/style.go`

**Interfaces:**
- Consumes: nothing (first TUI file)
- Produces:
  - `type tui.listItem struct { ID, Label, Color string }`
  - `type tui.itemSelectedMsg struct { Item listItem }`
  - `type tui.listBackMsg struct{}`
  - `type tui.listModel struct{...}`, `func tui.newListModel(title string, items []listItem, canCreate bool) listModel`, `func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd)`, `func (m listModel) View() string`
  - `type tui.confirmResultMsg struct { Confirmed bool }`
  - `type tui.confirmModel struct { Message string }`, `func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd)`, `func (m confirmModel) View() string`
  - `func tui.colorStyle(color string) lipgloss.Style`

- [ ] **Step 1: Add the Bubbletea/Lipgloss dependencies**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write the failing tests for listModel**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestListModel_MoveCursorDownAndUp(t *testing.T) {
	m := newListModel("Test", []listItem{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}}, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.Cursor != 1 {
		t.Fatalf("expected cursor 1 after one 'j', got %d", m.Cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // should clamp at last item
	if m.Cursor != 2 {
		t.Fatalf("expected cursor to clamp at 2, got %d", m.Cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.Cursor != 1 {
		t.Fatalf("expected cursor 1 after one 'k', got %d", m.Cursor)
	}
}

func TestListModel_EnterSelectsCurrentItem(t *testing.T) {
	m := newListModel("Test", []listItem{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, false)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command to be returned on enter")
	}
	msg := cmd()
	selected, ok := msg.(itemSelectedMsg)
	if !ok {
		t.Fatalf("expected itemSelectedMsg, got %T", msg)
	}
	if selected.Item.ID != "b" {
		t.Fatalf("expected item 'b' selected, got %q", selected.Item.ID)
	}
}

func TestListModel_EscSendsBackMsg(t *testing.T) {
	m := newListModel("Test", []listItem{{ID: "a", Label: "A"}}, false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command to be returned on esc")
	}
	if _, ok := cmd().(listBackMsg); !ok {
		t.Fatalf("expected listBackMsg, got %T", cmd())
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/tui/... -v`
Expected: FAIL — package/types undefined

- [ ] **Step 4: Implement style.go**

```go
package tui

import "github.com/charmbracelet/lipgloss"

func colorStyle(color string) lipgloss.Style {
	switch color {
	case "amarillo":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	case "rojo":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	case "verde":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	default:
		return lipgloss.NewStyle()
	}
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	cursorStyle   = lipgloss.NewStyle().Bold(true)
	helpHintStyle = lipgloss.NewStyle().Faint(true)
)
```

- [ ] **Step 5: Implement list.go**

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type listItem struct {
	ID    string
	Label string
	Color string
}

type itemSelectedMsg struct{ Item listItem }
type listBackMsg struct{}

type listModel struct {
	Title     string
	Items     []listItem
	Cursor    int
	CanCreate bool
}

func newListModel(title string, items []listItem, canCreate bool) listModel {
	return listModel{Title: title, Items: items, CanCreate: canCreate}
}

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.Cursor < len(m.Items)-1 {
			m.Cursor++
		}
	case "k", "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "enter":
		if len(m.Items) > 0 {
			item := m.Items[m.Cursor]
			return m, func() tea.Msg { return itemSelectedMsg{Item: item} }
		}
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m listModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.Title) + "\n\n")
	if len(m.Items) == 0 {
		b.WriteString("(vacío)\n")
	}
	for i, item := range m.Items {
		prefix := "  "
		if i == m.Cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + colorStyle(item.Color).Render(item.Label) + "\n")
	}
	if m.CanCreate {
		b.WriteString("\n" + helpHintStyle.Render("[a] nueva conexión"))
	}
	return b.String()
}
```

- [ ] **Step 6: Run listModel tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestListModel`
Expected: PASS

- [ ] **Step 7: Write the failing tests for confirmModel**

`internal/tui/confirm_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModel_YConfirms(t *testing.T) {
	m := confirmModel{Message: "¿Borrar documento?"}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command on 'y'")
	}
	result, ok := cmd().(confirmResultMsg)
	if !ok || !result.Confirmed {
		t.Fatalf("expected confirmResultMsg{Confirmed:true}, got %#v", cmd())
	}
}

func TestConfirmModel_NAndEscCancel(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("n")},
		{Type: tea.KeyEsc},
	} {
		m := confirmModel{Message: "¿Borrar documento?"}
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("expected a command on %v", key)
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok || result.Confirmed {
			t.Fatalf("expected confirmResultMsg{Confirmed:false} for %v, got %#v", key, cmd())
		}
	}
}
```

- [ ] **Step 8: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestConfirmModel`
Expected: FAIL — `confirmModel`/`confirmResultMsg` undefined

- [ ] **Step 9: Implement confirm.go**

```go
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type confirmResultMsg struct{ Confirmed bool }

type confirmModel struct {
	Message string
}

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y":
		return m, func() tea.Msg { return confirmResultMsg{Confirmed: true} }
	case "n", "esc":
		return m, func() tea.Msg { return confirmResultMsg{Confirmed: false} }
	}
	return m, nil
}

func (m confirmModel) View() string {
	return fmt.Sprintf("%s (y/n)", m.Message)
}
```

- [ ] **Step 10: Run all internal/tui tests**

Run: `go test ./internal/tui/... -v`
Expected: PASS (all listModel and confirmModel tests)

- [ ] **Step 11: Commit**

```bash
git add internal/tui/list.go internal/tui/list_test.go internal/tui/confirm.go internal/tui/confirm_test.go internal/tui/style.go go.mod go.sum
git commit -m "feat: add reusable list and confirm-modal Bubbletea components"
```

---

### Task 8: Connection picker + create-connection form

**Files:**
- Create: `internal/tui/connection_picker.go`
- Test: `internal/tui/connection_picker_test.go`

**Interfaces:**
- Consumes: `listModel`/`listItem`/`itemSelectedMsg`/`listBackMsg` (Task 7), `config.Connection`/`config.ListConnections`/`config.AddConnection` (Tasks 2–3)
- Produces:
  - `type tui.connectionChosenMsg struct { Conn config.Connection }`
  - `type tui.connectionPickerModel struct{...}`
  - `func tui.newConnectionPickerModel(conns []config.Connection) connectionPickerModel`
  - `func (m connectionPickerModel) Update(msg tea.Msg) (connectionPickerModel, tea.Cmd)`
  - `func (m connectionPickerModel) View() string`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
)

func TestConnectionPicker_SelectingItemSendsConnectionChosenMsg(t *testing.T) {
	conns := []config.Connection{
		{Name: "qa", URI: "mongodb://qa", Color: "verde"},
		{Name: "prod", URI: "mongodb://prod", Color: "rojo"},
	}
	m := newConnectionPickerModel(conns)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter")
	}
	msg := cmd()
	chosen, ok := msg.(connectionChosenMsg)
	if !ok {
		t.Fatalf("expected connectionChosenMsg, got %T", msg)
	}
	if chosen.Conn.Name != "qa" {
		t.Fatalf("expected 'qa' chosen, got %q", chosen.Conn.Name)
	}
}

func TestConnectionPicker_PressingAOpensCreateForm(t *testing.T) {
	m := newConnectionPickerModel(nil)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected picker to enter 'creating' mode after pressing 'a'")
	}
}

func TestConnectionPicker_CreateFormSubmitsNewConnection(t *testing.T) {
	m := newConnectionPickerModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	// name field
	for _, r := range "movatec-dev" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// uri field
	for _, r := range "mongodb://x:27017/y" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// color field: cycle to "rojo" (starts at "amarillo")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create form")
	}
	if got := m.form.name; got != "movatec-dev" {
		t.Fatalf("expected name 'movatec-dev', got %q", got)
	}
	if got := m.form.color; got != "rojo" {
		t.Fatalf("expected color 'rojo' after one 'l', got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestConnectionPicker`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement connection_picker.go**

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
)

type connectionChosenMsg struct{ Conn config.Connection }
type connectionCreatedMsg struct{ Conn config.Connection }
type connectionCreateErrMsg struct{ Err error }

var colorChoices = []string{"amarillo", "rojo", "verde"}

type connectionForm struct {
	name       string
	uri        string
	color      string
	field      int // 0=name, 1=uri, 2=color
}

func newConnectionForm() connectionForm {
	return connectionForm{color: colorChoices[0]}
}

func (f connectionForm) update(msg tea.KeyMsg) connectionForm {
	switch msg.String() {
	case "tab":
		f.field = (f.field + 1) % 3
		return f
	case "shift+tab":
		f.field = (f.field + 2) % 3
		return f
	}

	if f.field == 2 {
		switch msg.String() {
		case "l", "right":
			f.color = nextColor(f.color, 1)
		case "h", "left":
			f.color = nextColor(f.color, -1)
		}
		return f
	}

	switch msg.Type {
	case tea.KeyBackspace:
		if f.field == 0 && len(f.name) > 0 {
			f.name = f.name[:len(f.name)-1]
		} else if f.field == 1 && len(f.uri) > 0 {
			f.uri = f.uri[:len(f.uri)-1]
		}
	case tea.KeyRunes:
		text := string(msg.Runes)
		if f.field == 0 {
			f.name += text
		} else if f.field == 1 {
			f.uri += text
		}
	}
	return f
}

func nextColor(current string, delta int) string {
	idx := 0
	for i, c := range colorChoices {
		if c == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(colorChoices)) % len(colorChoices)
	return colorChoices[idx]
}

type connectionPickerModel struct {
	list     listModel
	creating bool
	form     connectionForm
	err      error
}

func newConnectionPickerModel(conns []config.Connection) connectionPickerModel {
	items := make([]listItem, len(conns))
	for i, c := range conns {
		items[i] = listItem{ID: c.Name, Label: c.Name, Color: c.Color}
	}
	return connectionPickerModel{list: newListModel("Conexiones", items, true)}
}

func (m connectionPickerModel) connectionByName(name string, conns []config.Connection) config.Connection {
	for _, c := range conns {
		if c.Name == name {
			return c
		}
	}
	return config.Connection{}
}

func (m connectionPickerModel) Update(msg tea.Msg) (connectionPickerModel, tea.Cmd) {
	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		if keyMsg.String() == "esc" {
			m.creating = false
			return m, nil
		}
		if keyMsg.String() == "enter" {
			conn := config.Connection{Name: m.form.name, URI: m.form.uri, Color: m.form.color}
			return m, func() tea.Msg {
				if err := config.AddConnection(conn); err != nil {
					return connectionCreateErrMsg{Err: err}
				}
				return connectionCreatedMsg{Conn: conn}
			}
		}
		m.form = m.form.update(keyMsg)
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "a" {
		m.creating = true
		m.form = newConnectionForm()
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if cmd == nil {
		return m, nil
	}
	if selected, ok := cmd().(itemSelectedMsg); ok {
		for _, item := range m.list.Items {
			if item.ID == selected.Item.ID {
				return m, func() tea.Msg {
					return connectionChosenMsg{Conn: config.Connection{Name: item.ID, Color: item.Color}}
				}
			}
		}
	}
	return m, cmd
}

func (m connectionPickerModel) View() string {
	if m.creating {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Nueva conexión") + "\n\n")
		b.WriteString("Nombre: " + m.form.name)
		if m.form.field == 0 {
			b.WriteString(" <")
		}
		b.WriteString("\nURI:    " + m.form.uri)
		if m.form.field == 1 {
			b.WriteString(" <")
		}
		b.WriteString("\nColor:  " + colorStyle(m.form.color).Render(m.form.color))
		if m.form.field == 2 {
			b.WriteString(" <")
		}
		b.WriteString("\n\n[Tab] siguiente campo  [h/l] cambiar color  [Enter] guardar  [Esc] cancelar")
		return b.String()
	}
	return m.list.View()
}
```

**Note:** `connectionChosenMsg` in the "not creating" branch above only carries `Name`/`Color` from the list item, not the resolved `URI` — the caller (Task 16's `RootModel`) is responsible for calling `config.ResolveConnection(name)` to get the full URI before connecting, since `connectionPickerModel` only holds display data (name+color), not URIs, to avoid keeping secrets in view state longer than necessary.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestConnectionPicker`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/connection_picker.go internal/tui/connection_picker_test.go
git commit -m "feat: add connection picker view with create-connection form"
```

---

### Task 9: Database list and collection list views

**Files:**
- Create: `internal/tui/db_collection.go`
- Test: `internal/tui/db_collection_test.go`

**Interfaces:**
- Consumes: `listModel`/`listItem`/`itemSelectedMsg`/`listBackMsg` (Task 7), `mongo.Client` (Task 4)
- Produces:
  - `type tui.databaseChosenMsg struct { Name string }`
  - `type tui.collectionChosenMsg struct { Name string }`
  - `func tui.newDatabaseListModel(names []string) listModel` (thin wrapper, returns a plain `listModel`)
  - `func tui.newCollectionListModel(names []string) listModel`
  - `func tui.translateDatabaseSelection(msg tea.Msg) (tea.Msg, bool)` and `func tui.translateCollectionSelection(msg tea.Msg) (tea.Msg, bool)` — helpers that turn a generic `itemSelectedMsg` from the shared `listModel` into the specific message type each view needs

- [ ] **Step 1: Write the failing tests**

```go
package tui

import "testing"

func TestNewDatabaseListModel_ListsGivenNames(t *testing.T) {
	m := newDatabaseListModel([]string{"admin", "haddacloud-v2"})
	if len(m.Items) != 2 || m.Items[0].Label != "admin" || m.Items[1].Label != "haddacloud-v2" {
		t.Fatalf("unexpected items: %+v", m.Items)
	}
}

func TestTranslateDatabaseSelection(t *testing.T) {
	msg := itemSelectedMsg{Item: listItem{ID: "admin", Label: "admin"}}
	translated, ok := translateDatabaseSelection(msg)
	if !ok {
		t.Fatal("expected translation to succeed for itemSelectedMsg")
	}
	chosen, ok := translated.(databaseChosenMsg)
	if !ok || chosen.Name != "admin" {
		t.Fatalf("expected databaseChosenMsg{Name:\"admin\"}, got %#v", translated)
	}

	_, ok = translateDatabaseSelection(listBackMsg{})
	if ok {
		t.Fatal("expected translation to fail for a non-selection message")
	}
}

func TestTranslateCollectionSelection(t *testing.T) {
	msg := itemSelectedMsg{Item: listItem{ID: "users", Label: "users"}}
	translated, ok := translateCollectionSelection(msg)
	if !ok {
		t.Fatal("expected translation to succeed for itemSelectedMsg")
	}
	chosen, ok := translated.(collectionChosenMsg)
	if !ok || chosen.Name != "users" {
		t.Fatalf("expected collectionChosenMsg{Name:\"users\"}, got %#v", translated)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run "TestNewDatabaseListModel|TestTranslate"`
Expected: FAIL — functions undefined

- [ ] **Step 3: Implement db_collection.go**

```go
package tui

import tea "github.com/charmbracelet/bubbletea"

type databaseChosenMsg struct{ Name string }
type collectionChosenMsg struct{ Name string }

func newDatabaseListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Bases de datos", items, false)
}

func newCollectionListModel(names []string) listModel {
	items := make([]listItem, len(names))
	for i, n := range names {
		items[i] = listItem{ID: n, Label: n}
	}
	return newListModel("Colecciones", items, false)
}

func translateDatabaseSelection(msg tea.Msg) (tea.Msg, bool) {
	selected, ok := msg.(itemSelectedMsg)
	if !ok {
		return nil, false
	}
	return databaseChosenMsg{Name: selected.Item.ID}, true
}

func translateCollectionSelection(msg tea.Msg) (tea.Msg, bool) {
	selected, ok := msg.(itemSelectedMsg)
	if !ok {
		return nil, false
	}
	return collectionChosenMsg{Name: selected.Item.ID}, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestNewDatabaseListModel|TestTranslate"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/db_collection.go internal/tui/db_collection_test.go
git commit -m "feat: add database and collection list views"
```

---

### Task 10: Document list view (table, pagination, filter)

**Files:**
- Create: `internal/tui/document_list.go`
- Test: `internal/tui/document_list_test.go`

**Interfaces:**
- Consumes: `mongo.Client`/`bson.M` (Task 4-5)
- Produces:
  - `type tui.documentChosenMsg struct { Doc bson.M }`
  - `type tui.docListModel struct{...}`
  - `func tui.newDocListModel(docs []bson.M, total int64, page int64, pageSize int64) docListModel`
  - `func (m docListModel) Update(msg tea.Msg) (docListModel, tea.Cmd)`
  - `func (m docListModel) View() string`
  - `func (m docListModel) FilterText() string` (exposes the current filter bar text so the parent can re-query Mongo)

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func sampleDocs() []bson.M {
	return []bson.M{
		{"_id": "1", "name": "Ana"},
		{"_id": "2", "name": "Beto"},
	}
}

func TestDocListModel_EnterOnRowSendsDocumentChosenMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter")
	}
	chosen, ok := cmd().(documentChosenMsg)
	if !ok {
		t.Fatalf("expected documentChosenMsg, got %T", cmd())
	}
	if chosen.Doc["_id"] != "2" {
		t.Fatalf("expected doc '2' chosen, got %+v", chosen.Doc)
	}
}

func TestDocListModel_SlashOpensFilterAndTypingUpdatesFilterText(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.filtering {
		t.Fatal("expected filtering mode to be active after '/'")
	}
	for _, r := range `{"name":"Ana"}` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.FilterText() != `{"name":"Ana"}` {
		t.Fatalf("expected filter text to accumulate, got %q", m.FilterText())
	}
}

func TestDocListModel_NextPageSendsPageChangedMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 50, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("expected a command on 'n'")
	}
	changed, ok := cmd().(pageChangedMsg)
	if !ok || changed.Page != 1 {
		t.Fatalf("expected pageChangedMsg{Page:1}, got %#v", cmd())
	}
}

func TestDocListModel_ISendsInsertRequestedMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if cmd == nil {
		t.Fatal("expected a command on 'i'")
	}
	if _, ok := cmd().(insertRequestedMsg); !ok {
		t.Fatalf("expected insertRequestedMsg, got %T", cmd())
	}
}

func TestDocListModel_TabSendsSwitchToIndexesMsg(t *testing.T) {
	m := newDocListModel(sampleDocs(), 2, 0, 20)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected a command on Tab")
	}
	if _, ok := cmd().(switchToIndexesMsg); !ok {
		t.Fatalf("expected switchToIndexesMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestDocListModel`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement document_list.go**

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type documentChosenMsg struct{ Doc bson.M }
type pageChangedMsg struct{ Page int64 }
type filterSubmittedMsg struct{ Filter string }
type insertRequestedMsg struct{}
type switchToIndexesMsg struct{}

type docListModel struct {
	docs      []bson.M
	total     int64
	page      int64
	pageSize  int64
	cursor    int
	filtering bool
	filter    string
}

func newDocListModel(docs []bson.M, total, page, pageSize int64) docListModel {
	return docListModel{docs: docs, total: total, page: page, pageSize: pageSize}
}

func (m docListModel) FilterText() string { return m.filter }

func (m docListModel) Update(msg tea.Msg) (docListModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.filtering {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.filtering = false
			filter := m.filter
			return m, func() tea.Msg { return filterSubmittedMsg{Filter: filter} }
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case tea.KeyRunes:
			m.filter += string(keyMsg.Runes)
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.docs)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.docs) > 0 {
			doc := m.docs[m.cursor]
			return m, func() tea.Msg { return documentChosenMsg{Doc: doc} }
		}
	case "/":
		m.filtering = true
		m.filter = ""
	case "n":
		if (m.page+1)*m.pageSize < m.total {
			page := m.page + 1
			return m, func() tea.Msg { return pageChangedMsg{Page: page} }
		}
	case "p":
		if m.page > 0 {
			page := m.page - 1
			return m, func() tea.Msg { return pageChangedMsg{Page: page} }
		}
	case "i", "a":
		return m, func() tea.Msg { return insertRequestedMsg{} }
	case "tab":
		return m, func() tea.Msg { return switchToIndexesMsg{} }
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m docListModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Documentos (%d total, página %d)", m.total, m.page+1)) + "\n\n")

	if m.filtering {
		b.WriteString("Filtro: " + m.filter + "_\n\n")
	} else if m.filter != "" {
		b.WriteString(helpHintStyle.Render("Filtro activo: "+m.filter) + "\n\n")
	}

	if len(m.docs) == 0 {
		b.WriteString("(sin documentos)\n")
	}
	for i, doc := range m.docs {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + fmt.Sprintf("%v\n", doc["_id"]))
	}
	b.WriteString("\n" + helpHintStyle.Render("[/] filtrar  [n/p] página  [Enter] ver  [i] insertar  [Tab] índices"))
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestDocListModel`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/document_list.go internal/tui/document_list_test.go
git commit -m "feat: add document list view with pagination and filter bar"
```

---

### Task 11: Document detail view

**Files:**
- Create: `internal/tui/document_detail.go`
- Test: `internal/tui/document_detail_test.go`

**Interfaces:**
- Consumes: `bson.M`, `listBackMsg` (Task 7)
- Produces:
  - `type tui.fieldSelectedMsg struct { Field string; Value any }`
  - `type tui.docDetailModel struct{...}`
  - `func tui.newDocDetailModel(doc bson.M) docDetailModel`
  - `func (m docDetailModel) Update(msg tea.Msg) (docDetailModel, tea.Cmd)`
  - `func (m docDetailModel) View() string`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDocDetailModel_EnterOnFieldSendsFieldSelectedMsg(t *testing.T) {
	m := newDocDetailModel(bson.M{"_id": "1", "age": int32(30), "name": "Ana"})

	// fields are sorted alphabetically: _id, age, name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // -> age

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd == nil {
		t.Fatal("expected a command on 'e'")
	}
	selected, ok := cmd().(fieldSelectedMsg)
	if !ok || selected.Field != "age" {
		t.Fatalf("expected fieldSelectedMsg{Field:\"age\"}, got %#v", cmd())
	}
}

func TestDocDetailModel_UppercaseESendsEditFullMsg(t *testing.T) {
	m := newDocDetailModel(bson.M{"_id": "1"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	if cmd == nil {
		t.Fatal("expected a command on 'E'")
	}
	if _, ok := cmd().(editFullRequestedMsg); !ok {
		t.Fatalf("expected editFullRequestedMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestDocDetailModel`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement document_detail.go**

```go
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fieldSelectedMsg struct {
	Field string
	Value any
}
type editFullRequestedMsg struct{}
type deleteRequestedMsg struct{}

type docDetailModel struct {
	doc    bson.M
	fields []string
	cursor int
}

func newDocDetailModel(doc bson.M) docDetailModel {
	fields := make([]string, 0, len(doc))
	for k := range doc {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return docDetailModel{doc: doc, fields: fields}
}

func (m docDetailModel) Update(msg tea.Msg) (docDetailModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.fields)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "e":
		if len(m.fields) > 0 {
			field := m.fields[m.cursor]
			value := m.doc[field]
			return m, func() tea.Msg { return fieldSelectedMsg{Field: field, Value: value} }
		}
	case "E":
		return m, func() tea.Msg { return editFullRequestedMsg{} }
	case "d", "x":
		return m, func() tea.Msg { return deleteRequestedMsg{} }
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m docDetailModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Documento") + "\n\n")
	for i, field := range m.fields {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix + fmt.Sprintf("%s: %v\n", field, m.doc[field]))
	}
	b.WriteString("\n" + helpHintStyle.Render("[e] editar campo  [E] editar completo  [d] borrar  [Esc] volver"))
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestDocDetailModel`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/document_detail.go internal/tui/document_detail_test.go
git commit -m "feat: add document detail view"
```

---

### Task 12: Inline field edit with confirmation

**Files:**
- Create: `internal/tui/field_edit.go`
- Test: `internal/tui/field_edit_test.go`

**Interfaces:**
- Consumes: `fieldSelectedMsg` (Task 11), `confirmModel`/`confirmResultMsg` (Task 7)
- Produces:
  - `type tui.fieldUpdateConfirmedMsg struct { Field string; NewValue string }`
  - `type tui.fieldEditModel struct{...}`
  - `func tui.newFieldEditModel(field string, currentValue any) fieldEditModel`
  - `func (m fieldEditModel) Update(msg tea.Msg) (fieldEditModel, tea.Cmd)`
  - `func (m fieldEditModel) View() string`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFieldEditModel_TypeThenEnterShowsConfirmation(t *testing.T) {
	m := newFieldEditModel("age", int32(30))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil}) // clear "30" prefill
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Runes: nil})
	for _, r := range "31" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.confirming {
		t.Fatal("expected fieldEditModel to enter confirming state after Enter")
	}
	if m.input != "31" {
		t.Fatalf("expected input '31', got %q", m.input)
	}
}

func TestFieldEditModel_ConfirmingYesSendsFieldUpdateConfirmedMsg(t *testing.T) {
	m := newFieldEditModel("age", int32(30))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // confirm with prefilled value

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming")
	}
	confirmed, ok := cmd().(fieldUpdateConfirmedMsg)
	if !ok || confirmed.Field != "age" || confirmed.NewValue != "30" {
		t.Fatalf("expected fieldUpdateConfirmedMsg{Field:\"age\",NewValue:\"30\"}, got %#v", cmd())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestFieldEditModel`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement field_edit.go**

```go
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type fieldUpdateConfirmedMsg struct {
	Field    string
	NewValue string
}

type fieldEditModel struct {
	field      string
	input      string
	confirming bool
	confirm    confirmModel
}

func newFieldEditModel(field string, currentValue any) fieldEditModel {
	return fieldEditModel{field: field, input: fmt.Sprintf("%v", currentValue)}
}

func (m fieldEditModel) Update(msg tea.Msg) (fieldEditModel, tea.Cmd) {
	if m.confirming {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		if result, ok := cmd().(confirmResultMsg); ok {
			if !result.Confirmed {
				m.confirming = false
				return m, nil
			}
			field, value := m.field, m.input
			return m, func() tea.Msg { return fieldUpdateConfirmedMsg{Field: field, NewValue: value} }
		}
		return m, cmd
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.Type {
	case tea.KeyEnter:
		m.confirming = true
		m.confirm = confirmModel{Message: fmt.Sprintf("¿Actualizar %q a %q?", m.field, m.input)}
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	case tea.KeyEsc:
		return m, func() tea.Msg { return listBackMsg{} }
	case tea.KeyRunes:
		m.input += string(keyMsg.Runes)
	}
	return m, nil
}

func (m fieldEditModel) View() string {
	if m.confirming {
		return m.confirm.View()
	}
	return fmt.Sprintf("Editar %s: %s_\n\n[Enter] confirmar  [Esc] cancelar", m.field, m.input)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestFieldEditModel`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/field_edit.go internal/tui/field_edit_test.go
git commit -m "feat: add inline field edit with confirmation"
```

---

### Task 13: Full-document edit via $EDITOR, and insert (reusing the same mechanism)

**Files:**
- Create: `internal/tui/editor.go`
- Test: `internal/tui/editor_test.go`

**Interfaces:**
- Consumes: `bson.M`
- Produces:
  - `func tui.editorCommand() string` (resolves `$EDITOR`, falls back to `"nvim"`)
  - `func tui.writeDocToTempFile(doc bson.M) (path string, cleanup func(), err error)`
  - `func tui.readDocFromTempFile(path string) (bson.M, error)`
  - `func tui.buildEditorCmd(path string) *exec.Cmd`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestEditorCommand_UsesEnvVarWhenSet(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")
	if got := editorCommand(); got != "code --wait" {
		t.Fatalf("expected 'code --wait', got %q", got)
	}
}

func TestEditorCommand_FallsBackToNvim(t *testing.T) {
	t.Setenv("EDITOR", "")
	if got := editorCommand(); got != "nvim" {
		t.Fatalf("expected fallback 'nvim', got %q", got)
	}
}

func TestWriteAndReadDocToTempFile_RoundTrips(t *testing.T) {
	doc := bson.M{"_id": "1", "name": "Ana", "age": int32(30)}

	path, cleanup, err := writeDocToTempFile(doc)
	if err != nil {
		t.Fatalf("writeDocToTempFile failed: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected temp file to exist: %v", err)
	}

	got, err := readDocFromTempFile(path)
	if err != nil {
		t.Fatalf("readDocFromTempFile failed: %v", err)
	}
	if got["name"] != "Ana" {
		t.Fatalf("expected name 'Ana' after round-trip, got %+v", got)
	}
}

func TestReadDocFromTempFile_InvalidJSONReturnsError(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.json")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString("not json"); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	f.Close()

	if _, err := readDocFromTempFile(f.Name()); err == nil {
		t.Fatal("expected an error for invalid JSON content")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run "TestEditorCommand|TestWriteAndReadDocToTempFile|TestReadDocFromTempFile"`
Expected: FAIL — functions undefined

- [ ] **Step 3: Implement editor.go**

```go
package tui

import (
	"fmt"
	"os"
	"os/exec"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func editorCommand() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "nvim"
}

func writeDocToTempFile(doc bson.M) (string, func(), error) {
	data, err := bson.MarshalExtJSONIndent(doc, false, false, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("serializando documento: %w", err)
	}

	f, err := os.CreateTemp("", "lazymongo-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("creando archivo temporal: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", nil, fmt.Errorf("escribiendo archivo temporal: %w", err)
	}
	f.Close()

	cleanup := func() { os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}

func readDocFromTempFile(path string) (bson.M, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo archivo temporal: %w", err)
	}
	var doc bson.M
	if err := bson.UnmarshalExtJSON(data, false, &doc); err != nil {
		return nil, fmt.Errorf("el archivo no contiene JSON válido: %w", err)
	}
	return doc, nil
}

func buildEditorCmd(path string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", editorCommand()+" \""+path+"\"")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run "TestEditorCommand|TestWriteAndReadDocToTempFile|TestReadDocFromTempFile"`
Expected: PASS

- [ ] **Step 5: Write the failing test for the Bubbletea suspend/resume message flow**

```go
func TestEditFullRequestedMsg_TriggersExecProcessCmd(t *testing.T) {
	doc := bson.M{"_id": "1", "name": "Ana"}
	cmd := startEditFullFlow(doc)
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd from startEditFullFlow")
	}
	// startEditFullFlow must return a tea.ExecProcess-wrapped command; we only
	// assert it's non-nil here since tea.ExecProcess's returned Cmd type is
	// opaque and meant to be driven by the Bubbletea runtime, not called directly
	// in a unit test.
}
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestEditFullRequestedMsg`
Expected: FAIL — `startEditFullFlow` undefined

- [ ] **Step 7: Implement startEditFullFlow in editor.go**

Append to `internal/tui/editor.go`:

```go
type editFullDoneMsg struct {
	Doc bson.M
	Err error
}

// startEditFullFlow writes doc to a temp file, suspends the Bubbletea
// screen to run $EDITOR against it, and on return parses the result.
// Modeled on how lazygit shells out to an external editor.
func startEditFullFlow(doc bson.M) tea.Cmd {
	path, cleanup, err := writeDocToTempFile(doc)
	if err != nil {
		return func() tea.Msg { return editFullDoneMsg{Err: err} }
	}

	cmd := buildEditorCmd(path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer cleanup()
		if err != nil {
			return editFullDoneMsg{Err: fmt.Errorf("el editor terminó con error: %w", err)}
		}
		edited, err := readDocFromTempFile(path)
		if err != nil {
			return editFullDoneMsg{Err: err}
		}
		return editFullDoneMsg{Doc: edited}
	})
}
```

Add the missing import at the top of `internal/tui/editor.go`:
```go
import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS (all internal/tui tests)

- [ ] **Step 9: Commit**

```bash
git add internal/tui/editor.go internal/tui/editor_test.go
git commit -m "feat: add full-document edit via \$EDITOR using tea.ExecProcess"
```

---

### Task 14: Delete document flow

**Files:**
- Create: `internal/tui/delete.go`
- Test: `internal/tui/delete_test.go`

**Interfaces:**
- Consumes: `deleteRequestedMsg` (Task 11), `confirmModel`/`confirmResultMsg` (Task 7)
- Produces:
  - `type tui.deleteConfirmedMsg struct{}`
  - `type tui.deleteFlowModel struct{...}`
  - `func tui.newDeleteFlowModel(docID any) deleteFlowModel`
  - `func (m deleteFlowModel) Update(msg tea.Msg) (deleteFlowModel, tea.Cmd)`
  - `func (m deleteFlowModel) View() string`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDeleteFlowModel_YSendsDeleteConfirmedMsg(t *testing.T) {
	m := newDeleteFlowModel("doc-1")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command on 'y'")
	}
	if _, ok := cmd().(deleteConfirmedMsg); !ok {
		t.Fatalf("expected deleteConfirmedMsg, got %T", cmd())
	}
}

func TestDeleteFlowModel_NSendsListBackMsg(t *testing.T) {
	m := newDeleteFlowModel("doc-1")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("expected a command on 'n'")
	}
	if _, ok := cmd().(listBackMsg); !ok {
		t.Fatalf("expected listBackMsg (cancel), got %T", cmd())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestDeleteFlowModel`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement delete.go**

```go
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type deleteConfirmedMsg struct{}

type deleteFlowModel struct {
	docID   any
	confirm confirmModel
}

func newDeleteFlowModel(docID any) deleteFlowModel {
	return deleteFlowModel{
		docID:   docID,
		confirm: confirmModel{Message: fmt.Sprintf("¿Borrar el documento %v? Esta acción no se puede deshacer", docID)},
	}
}

func (m deleteFlowModel) Update(msg tea.Msg) (deleteFlowModel, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	if cmd == nil {
		return m, nil
	}
	result, ok := cmd().(confirmResultMsg)
	if !ok {
		return m, cmd
	}
	if result.Confirmed {
		return m, func() tea.Msg { return deleteConfirmedMsg{} }
	}
	return m, func() tea.Msg { return listBackMsg{} }
}

func (m deleteFlowModel) View() string {
	return m.confirm.View()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestDeleteFlowModel`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/delete.go internal/tui/delete_test.go
git commit -m "feat: add delete document confirmation flow"
```

---

### Task 15: Index list view (view, create, drop)

**Files:**
- Create: `internal/tui/index_list.go`
- Test: `internal/tui/index_list_test.go`

**Interfaces:**
- Consumes: `mongo.IndexInfo` (Task 4), `confirmModel`/`confirmResultMsg` (Task 7)
- Produces:
  - `type tui.indexCreateSubmittedMsg struct { KeysJSON string; Unique bool }`
  - `type tui.indexDropConfirmedMsg struct { Name string }`
  - `type tui.idxListModel struct{...}`
  - `func tui.newIdxListModel(indexes []mongo.IndexInfo) idxListModel`
  - `func (m idxListModel) Update(msg tea.Msg) (idxListModel, tea.Cmd)`
  - `func (m idxListModel) View() string`

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func sampleIndexes() []mongo.IndexInfo {
	return []mongo.IndexInfo{
		{Name: "_id_", Key: bson.M{"_id": 1}, Unique: true},
		{Name: "email_1", Key: bson.M{"email": 1}, Unique: true},
	}
}

func TestIdxListModel_DSendsIndexDropConfirmedFlow(t *testing.T) {
	m := newIdxListModel(sampleIndexes())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // -> email_1

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.confirmingDrop {
		t.Fatal("expected idxListModel to enter confirmingDrop state after 'd'")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a command after confirming drop")
	}
	dropped, ok := cmd().(indexDropConfirmedMsg)
	if !ok || dropped.Name != "email_1" {
		t.Fatalf("expected indexDropConfirmedMsg{Name:\"email_1\"}, got %#v", cmd())
	}
}

func TestIdxListModel_AOpensCreateFormAndEnterSubmits(t *testing.T) {
	m := newIdxListModel(nil)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.creating {
		t.Fatal("expected idxListModel to enter creating state after 'a'")
	}

	for _, r := range `{"email":1}` {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // move to unique toggle
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // toggle unique on

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on submitting the create-index form")
	}
	submitted, ok := cmd().(indexCreateSubmittedMsg)
	if !ok || submitted.KeysJSON != `{"email":1}` || !submitted.Unique {
		t.Fatalf("expected indexCreateSubmittedMsg{KeysJSON:'{\"email\":1}',Unique:true}, got %#v", cmd())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestIdxListModel`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement index_list.go**

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
)

type indexCreateSubmittedMsg struct {
	KeysJSON string
	Unique   bool
}
type indexDropConfirmedMsg struct{ Name string }

type idxListModel struct {
	indexes        []mongo.IndexInfo
	cursor         int
	confirmingDrop bool
	confirm        confirmModel
	creating       bool
	createKeys     string
	createUnique   bool
	createField    int // 0=keys, 1=unique toggle
}

func newIdxListModel(indexes []mongo.IndexInfo) idxListModel {
	return idxListModel{indexes: indexes}
}

func (m idxListModel) Update(msg tea.Msg) (idxListModel, tea.Cmd) {
	if m.confirmingDrop {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if cmd == nil {
			return m, nil
		}
		result, ok := cmd().(confirmResultMsg)
		if !ok {
			return m, cmd
		}
		m.confirmingDrop = false
		if result.Confirmed {
			name := m.indexes[m.cursor].Name
			return m, func() tea.Msg { return indexDropConfirmedMsg{Name: name} }
		}
		return m, nil
	}

	if m.creating {
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.creating = false
		case "tab":
			m.createField = (m.createField + 1) % 2
		case "enter":
			keys, unique := m.createKeys, m.createUnique
			return m, func() tea.Msg { return indexCreateSubmittedMsg{KeysJSON: keys, Unique: unique} }
		default:
			if m.createField == 1 && keyMsg.String() == " " {
				m.createUnique = !m.createUnique
			} else if m.createField == 0 {
				switch keyMsg.Type {
				case tea.KeyBackspace:
					if len(m.createKeys) > 0 {
						m.createKeys = m.createKeys[:len(m.createKeys)-1]
					}
				case tea.KeyRunes:
					m.createKeys += string(keyMsg.Runes)
				}
			}
		}
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.indexes)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "a":
		m.creating = true
		m.createKeys = ""
		m.createUnique = false
		m.createField = 0
	case "d":
		if len(m.indexes) > 0 {
			m.confirmingDrop = true
			m.confirm = confirmModel{Message: fmt.Sprintf("¿Borrar el índice %q?", m.indexes[m.cursor].Name)}
		}
	case "esc", "h":
		return m, func() tea.Msg { return listBackMsg{} }
	}
	return m, nil
}

func (m idxListModel) View() string {
	if m.confirmingDrop {
		return m.confirm.View()
	}
	if m.creating {
		unique := "no"
		if m.createUnique {
			unique = "sí"
		}
		return fmt.Sprintf(
			"Nuevo índice\n\nCampos (JSON, ej. {\"email\":1}): %s_\nUnique: %s\n\n[Tab] siguiente campo  [Espacio] alternar unique  [Enter] crear  [Esc] cancelar",
			m.createKeys, unique,
		)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Índices") + "\n\n")
	if len(m.indexes) == 0 {
		b.WriteString("(sin índices además de _id_)\n")
	}
	for i, idx := range m.indexes {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		unique := ""
		if idx.Unique {
			unique = " (unique)"
		}
		b.WriteString(fmt.Sprintf("%s%s %v%s\n", prefix, idx.Name, idx.Key, unique))
	}
	b.WriteString("\n" + helpHintStyle.Render("[a] crear índice  [d] borrar  [Esc] volver"))
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestIdxListModel`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/index_list.go internal/tui/index_list_test.go
git commit -m "feat: add index list view with create/drop flows"
```

---

### Task 16: Root model wiring, help overlay, and main.go

**Files:**
- Create: `internal/tui/help.go`
- Create: `internal/tui/root.go`
- Test: `internal/tui/root_test.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: every message/model type from Tasks 7–15, `mongo.Client` (Task 4), `config.Connection`/`config.ResolveConnection`/`config.ListConnections` (Tasks 2–3)
- Produces:
  - `type tui.RootModel struct{...}` implementing `tea.Model` (`Init`, `Update`, `View`)
  - `func tui.NewRootModel(client mongo.Client, resolved *config.Connection) RootModel` — `resolved` is nil when launched with no argument (shows the picker first)
  - `func tui.Run(client mongo.Client, resolved *config.Connection) error`

- [ ] **Step 1: Write the failing tests for RootModel navigation using the fake client**

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func newTestRootModel() (*RootModel, *mongo.FakeClient) {
	fake := mongo.NewFakeClient()
	fake.Databases["shop"] = map[string][]bson.M{
		"orders": {{"_id": "o1", "total": int32(10)}},
	}
	conn := config.Connection{Name: "qa", URI: "mongodb://fake", Color: "verde"}
	m := NewRootModel(fake, &conn)
	return &m, fake
}

func TestRootModel_InitConnectsAndLoadsDatabases(t *testing.T) {
	m, _ := newTestRootModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command that connects and lists databases")
	}
	msg := cmd()
	newModel, _ := m.Update(msg)
	root := newModel.(RootModel)
	if root.view != viewDatabaseList {
		t.Fatalf("expected view to become viewDatabaseList after connecting, got %v", root.view)
	}
}

func TestRootModel_FullDrillDownToDocumentDetail(t *testing.T) {
	// newTestRootModel's fake has exactly one database ("shop") and one
	// collection ("orders"), so pressing Enter always drills into that
	// single item — this simulates real key presses rather than
	// constructing itemSelectedMsg/documentChosenMsg by hand, since those
	// message types are only ever produced internally by listModel/docListModel
	// in response to an actual tea.KeyMsg, never accepted as external input.
	m, _ := newTestRootModel()

	cmd := m.Init()
	model, _ := m.Update(cmd())
	root := model.(RootModel)

	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewCollectionList {
		t.Fatalf("expected viewCollectionList, got %v", root.view)
	}

	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewDocumentList {
		t.Fatalf("expected viewDocumentList, got %v", root.view)
	}

	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	if root.view != viewDocumentDetail {
		t.Fatalf("expected viewDocumentDetail, got %v", root.view)
	}
	_ = cmd
}

func TestRootModel_QuitsOnCtrlC(t *testing.T) {
	m, _ := newTestRootModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a quit command on Ctrl+C")
	}
}

func TestRootModel_ViewShowsConnectionNameInStatusBar(t *testing.T) {
	m, _ := newTestRootModel()
	view := m.View()
	if !strings.Contains(view, "qa") {
		t.Fatalf("expected status bar to show connection name 'qa', got view:\n%s", view)
	}
}

// rootModelInDocList drives a RootModel from init through selecting the
// "shop" database and "orders" collection, landing in viewDocumentList.
func rootModelInDocList(t *testing.T) (RootModel, *mongo.FakeClient) {
	t.Helper()
	m, fake := newTestRootModel()

	// newTestRootModel's fake has exactly one database and one collection,
	// so pressing Enter twice always drills into "shop" then "orders" —
	// see the comment on TestRootModel_FullDrillDownToDocumentDetail for
	// why real key presses are used instead of constructing itemSelectedMsg
	// by hand.
	model, _ := m.Update(m.Init()())
	root := model.(RootModel)
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	model, cmd = root.Update(tea.KeyMsg{Type: tea.KeyEnter})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewDocumentList {
		t.Fatalf("setup failed: expected viewDocumentList, got %v", root.view)
	}
	return root, fake
}

// switchToIndexList drives a RootModel from viewDocumentList to viewIndexList
// by pressing Tab. This takes three Update calls, not one: Tab makes
// docListModel emit switchToIndexesMsg (1), which RootModel turns into an
// async m.loadIndexes() command (2), whose result (indexesLoadedMsg) is what
// actually sets m.view = viewIndexList (3).
func switchToIndexList(t *testing.T, root RootModel) RootModel {
	t.Helper()
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyTab})
	root = model.(RootModel)
	model, cmd = root.Update(cmd())
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)
	if root.view != viewIndexList {
		t.Fatalf("switchToIndexList setup failed: expected viewIndexList, got %v", root.view)
	}
	return root
}

func TestRootModel_InsertGoesThroughConfirmationBeforeWriting(t *testing.T) {
	root, fake := rootModelInDocList(t)

	model, _ := root.Update(insertRequestedMsg{})
	root = model.(RootModel)
	if root.editMode != "insert" {
		t.Fatalf("expected editMode 'insert', got %q", root.editMode)
	}

	// simulate the editor returning a new document (skipping the real
	// tea.ExecProcess invocation, which needs a real terminal/editor)
	model, _ = root.Update(editFullDoneMsg{Doc: bson.M{"total": int32(99)}})
	root = model.(RootModel)
	if root.view != viewConfirmWrite {
		t.Fatalf("expected viewConfirmWrite after editFullDoneMsg, got %v", root.view)
	}

	before := len(fake.Databases["shop"]["orders"])
	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	root = model.(RootModel)
	model, _ = root.Update(cmd())
	root = model.(RootModel)

	if len(fake.Databases["shop"]["orders"]) != before+1 {
		t.Fatalf("expected InsertOne to add a document, got %d (was %d)", len(fake.Databases["shop"]["orders"]), before)
	}
}

func TestRootModel_InsertCancelledDoesNotWrite(t *testing.T) {
	root, fake := rootModelInDocList(t)

	model, _ := root.Update(insertRequestedMsg{})
	root = model.(RootModel)
	model, _ = root.Update(editFullDoneMsg{Doc: bson.M{"total": int32(99)}})
	root = model.(RootModel)

	before := len(fake.Databases["shop"]["orders"])
	model, _ = root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	root = model.(RootModel)

	if len(fake.Databases["shop"]["orders"]) != before {
		t.Fatalf("expected no document added after cancelling, got %d (was %d)", len(fake.Databases["shop"]["orders"]), before)
	}
	if root.view != viewDocumentList {
		t.Fatalf("expected to return to viewDocumentList after cancelling, got %v", root.view)
	}
}

func TestRootModel_TabSwitchesToIndexList(t *testing.T) {
	root, fake := rootModelInDocList(t)
	fake.Indexes["shop"] = map[string][]mongo.IndexInfo{
		"orders": {{Name: "_id_", Key: bson.M{"_id": 1}, Unique: true}},
	}

	root = switchToIndexList(t, root)

	if root.view != viewIndexList {
		t.Fatalf("expected viewIndexList after Tab, got %v", root.view)
	}
	if len(root.idxList.indexes) != 1 {
		t.Fatalf("expected 1 index loaded, got %d", len(root.idxList.indexes))
	}
}

func TestRootModel_CreateIndexGoesThroughConfirmationBeforeWriting(t *testing.T) {
	root, fake := rootModelInDocList(t)
	root = switchToIndexList(t, root)

	model, _ := root.Update(indexCreateSubmittedMsg{KeysJSON: `{"total":1}`, Unique: false})
	root = model.(RootModel)
	if root.view != viewConfirmWrite {
		t.Fatalf("expected viewConfirmWrite after indexCreateSubmittedMsg, got %v", root.view)
	}

	model, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	root = model.(RootModel)
	model, _ = root.Update(cmd())

	if len(fake.Indexes["shop"]["orders"]) != 1 {
		t.Fatalf("expected 1 index created, got %d", len(fake.Indexes["shop"]["orders"]))
	}
}

func TestRootModel_DropIndexWritesImmediately(t *testing.T) {
	root, fake := rootModelInDocList(t)
	fake.Indexes["shop"] = map[string][]mongo.IndexInfo{
		"orders": {{Name: "total_1", Key: bson.M{"total": 1}}},
	}
	root = switchToIndexList(t, root)

	// idxListModel already confirms internally before emitting
	// indexDropConfirmedMsg, so RootModel executes it directly.
	model, cmd := root.Update(indexDropConfirmedMsg{Name: "total_1"})
	root = model.(RootModel)
	_, _ = root.Update(cmd())

	if len(fake.Indexes["shop"]["orders"]) != 0 {
		t.Fatalf("expected index dropped, got %+v", fake.Indexes["shop"]["orders"])
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/... -v -run TestRootModel`
Expected: FAIL — `RootModel`/`NewRootModel`/`viewDatabaseList`/etc. undefined

- [ ] **Step 3: Implement help.go**

```go
package tui

const helpText = `lazymongo — atajos

j/k, flechas   moverse
Enter          entrar / seleccionar
Esc, h         volver un nivel
/              filtrar (en Documentos)
Tab            alternar Documentos/Índices
n/p            página siguiente/anterior
i, a           insertar documento / crear conexión o índice
e              editar campo (en detalle de documento)
E              editar documento completo en $EDITOR
d, x           borrar (siempre pide confirmación)
?              esta ayuda
q, Ctrl+c      salir

Presiona cualquier tecla para cerrar esta ayuda.`

type helpModel struct{}

func (m helpModel) View() string { return helpText }
```

- [ ] **Step 4: Implement root.go**

```go
package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type viewID int

const (
	viewConnectionPicker viewID = iota
	viewDatabaseList
	viewCollectionList
	viewDocumentList
	viewDocumentDetail
	viewFieldEdit
	viewIndexList
	viewDelete
	viewConfirmWrite
	viewHelp
)

const pageSize = 20

type RootModel struct {
	client mongo.Client

	view      viewID
	prevViews []viewID

	conn config.Connection
	db   string
	coll string
	page int64

	connPicker connectionPickerModel
	dbList     listModel
	collList   listModel
	docList    docListModel
	docDetail  docDetailModel
	fieldEdit  fieldEditModel
	idxList    idxListModel
	delete     deleteFlowModel

	// pending write awaiting confirmation via viewConfirmWrite
	confirmWrite         confirmModel
	editMode             string // "edit" | "insert", set before startEditFullFlow
	pendingDoc           bson.M
	pendingIndexKeysJSON string
	pendingIndexUnique   bool

	err error
}

// NewRootModel builds the root model. If resolved is nil, the app starts on
// the connection picker; otherwise it connects immediately.
func NewRootModel(client mongo.Client, resolved *config.Connection) RootModel {
	m := RootModel{client: client}
	if resolved != nil {
		m.conn = *resolved
		m.view = viewDatabaseList
	} else {
		m.view = viewConnectionPicker
	}
	return m
}

func (m RootModel) pushView(v viewID) RootModel {
	m.prevViews = append(m.prevViews, m.view)
	m.view = v
	return m
}

func (m RootModel) popView() RootModel {
	if len(m.prevViews) == 0 {
		return m
	}
	m.view = m.prevViews[len(m.prevViews)-1]
	m.prevViews = m.prevViews[:len(m.prevViews)-1]
	return m
}

func (m RootModel) Init() tea.Cmd {
	if m.view != viewDatabaseList {
		return nil
	}
	return m.connectAndListDatabases()
}

type connectedMsg struct {
	Databases []string
	Err       error
}

func (m RootModel) connectAndListDatabases() tea.Cmd {
	client := m.client
	uri := m.conn.URI
	return func() tea.Msg {
		ctx := context.Background()
		if err := client.Connect(ctx, uri); err != nil {
			return connectedMsg{Err: err}
		}
		names, err := client.ListDatabases(ctx)
		if err != nil {
			return connectedMsg{Err: err}
		}
		return connectedMsg{Databases: names}
	}
}

type collectionsLoadedMsg struct {
	Collections []string
	Err         error
}

func (m RootModel) loadCollections() tea.Cmd {
	client, db := m.client, m.db
	return func() tea.Msg {
		names, err := client.ListCollections(context.Background(), db)
		return collectionsLoadedMsg{Collections: names, Err: err}
	}
}

type documentsLoadedMsg struct {
	Docs  []bson.M
	Total int64
	Err   error
}

func (m RootModel) loadDocuments(filter bson.M) tea.Cmd {
	client, db, coll, page := m.client, m.db, m.coll, m.page
	return func() tea.Msg {
		ctx := context.Background()
		docs, err := client.Find(ctx, db, coll, filter, page*pageSize, pageSize)
		if err != nil {
			return documentsLoadedMsg{Err: err}
		}
		total, err := client.CountDocuments(ctx, db, coll, filter)
		if err != nil {
			return documentsLoadedMsg{Err: err}
		}
		return documentsLoadedMsg{Docs: docs, Total: total}
	}
}

type indexesLoadedMsg struct {
	Indexes []mongo.IndexInfo
	Err     error
}

func (m RootModel) loadIndexes() tea.Cmd {
	client, db, coll := m.client, m.db, m.coll
	return func() tea.Msg {
		idxs, err := client.ListIndexes(context.Background(), db, coll)
		return indexesLoadedMsg{Indexes: idxs, Err: err}
	}
}

// docWriteCompletedMsg/indexWriteCompletedMsg report the result of a write
// that already went through the viewConfirmWrite confirmation step.
type docWriteCompletedMsg struct{ Err error }
type indexWriteCompletedMsg struct{ Err error }

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			if m.view != viewHelp {
				m = m.pushView(viewHelp)
				return m, nil
			}
		}
		if m.view == viewHelp {
			m = m.popView()
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case connectedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		items := make([]listItem, len(msg.Databases))
		for i, n := range msg.Databases {
			items[i] = listItem{ID: n, Label: n}
		}
		m.dbList = newDatabaseListModel(msg.Databases)
		m.view = viewDatabaseList
		return m, nil

	case connectionChosenMsg:
		conn, err := config.ResolveConnection(msg.Conn.Name)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.conn = conn
		return m, m.connectAndListDatabases()

	case connectionCreatedMsg, connectionCreateErrMsg:
		// re-render the picker with the latest connections list
		conns, err := config.ListConnections()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.connPicker = newConnectionPickerModel(conns)
		return m, nil

	case collectionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.collList = newCollectionListModel(msg.Collections)
		m.view = viewCollectionList
		return m, nil

	case documentsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.docList = newDocListModel(msg.Docs, msg.Total, m.page, pageSize)
		m.view = viewDocumentList
		return m, nil

	case indexesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.idxList = newIdxListModel(msg.Indexes)
		m.view = viewIndexList
		return m, nil

	case fieldSelectedMsg:
		m.fieldEdit = newFieldEditModel(msg.Field, msg.Value)
		m = m.pushView(viewFieldEdit)
		return m, nil

	case deleteRequestedMsg:
		id := m.docDetail.doc["_id"]
		m.delete = newDeleteFlowModel(id)
		m = m.pushView(viewDelete)
		return m, nil

	case deleteConfirmedMsg:
		id := m.docDetail.doc["_id"]
		client, db, coll := m.client, m.db, m.coll
		m = m.popView()
		return m, func() tea.Msg {
			err := client.DeleteOne(context.Background(), db, coll, id)
			return documentsLoadedMsg{Err: err}
		}

	case fieldUpdateConfirmedMsg:
		client, db, coll, id := m.client, m.db, m.coll, m.docDetail.doc["_id"]
		field, value := msg.Field, msg.NewValue
		m = m.popView()
		return m, func() tea.Msg {
			err := client.UpdateField(context.Background(), db, coll, id, field, value)
			return documentsLoadedMsg{Err: err}
		}

	case insertRequestedMsg:
		m.editMode = "insert"
		return m, startEditFullFlow(bson.M{})

	case editFullRequestedMsg:
		m.editMode = "edit"
		return m, startEditFullFlow(m.docDetail.doc)

	case editFullDoneMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.pendingDoc = msg.Doc
		m.confirmWrite = confirmModel{Message: "¿Guardar estos cambios en Mongo?"}
		m = m.pushView(viewConfirmWrite)
		return m, nil

	case switchToIndexesMsg:
		return m, m.loadIndexes()

	case indexCreateSubmittedMsg:
		m.pendingIndexKeysJSON = msg.KeysJSON
		m.pendingIndexUnique = msg.Unique
		m.confirmWrite = confirmModel{Message: "¿Crear este índice?"}
		m = m.pushView(viewConfirmWrite)
		return m, nil

	case indexDropConfirmedMsg:
		client, db, coll, name := m.client, m.db, m.coll, msg.Name
		return m, func() tea.Msg {
			err := client.DropIndex(context.Background(), db, coll, name)
			return indexWriteCompletedMsg{Err: err}
		}

	case docWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.pendingDoc = nil
		return m, m.loadDocuments(bson.M{})

	case indexWriteCompletedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, m.loadIndexes()

	case listBackMsg:
		switch m.view {
		case viewDatabaseList:
			m.view = viewConnectionPicker
		case viewCollectionList:
			m.view = viewDatabaseList
		case viewDocumentList:
			m.view = viewCollectionList
		case viewIndexList:
			m.view = viewDocumentList
		default:
			m = m.popView()
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.view {
	case viewConnectionPicker:
		m.connPicker, cmd = m.connPicker.Update(msg)
	case viewDatabaseList:
		var listCmd tea.Cmd
		m.dbList, listCmd = m.dbList.Update(msg)
		if listCmd != nil {
			if translated, ok := translateDatabaseSelection(listCmd()); ok {
				if chosen, ok := translated.(databaseChosenMsg); ok {
					m.db = chosen.Name
					return m, m.loadCollections()
				}
			}
			return m, listCmd
		}
	case viewCollectionList:
		var listCmd tea.Cmd
		m.collList, listCmd = m.collList.Update(msg)
		if listCmd != nil {
			if translated, ok := translateCollectionSelection(listCmd()); ok {
				if chosen, ok := translated.(collectionChosenMsg); ok {
					m.coll = chosen.Name
					m.page = 0
					return m, m.loadDocuments(bson.M{})
				}
			}
			return m, listCmd
		}
	case viewDocumentList:
		m.docList, cmd = m.docList.Update(msg)
		if cmd != nil {
			switch out := cmd().(type) {
			case pageChangedMsg:
				m.page = out.Page
				return m, m.loadDocuments(bson.M{})
			case filterSubmittedMsg:
				m.page = 0
				var filter bson.M
				if err := bson.UnmarshalExtJSON([]byte(out.Filter), false, &filter); err != nil {
					m.err = fmt.Errorf("filtro inválido: %w", err)
					return m, nil
				}
				return m, m.loadDocuments(filter)
			case documentChosenMsg:
				m.docDetail = newDocDetailModel(out.Doc)
				m = m.pushView(viewDocumentDetail)
				return m, nil
			}
			return m, cmd
		}
	case viewDocumentDetail:
		var docCmd tea.Cmd
		m.docDetail, docCmd = m.docDetail.Update(msg)
		if docCmd != nil {
			return m.dispatch(docCmd())
		}
	case viewFieldEdit:
		var feCmd tea.Cmd
		m.fieldEdit, feCmd = m.fieldEdit.Update(msg)
		if feCmd != nil {
			return m.dispatch(feCmd())
		}
	case viewIndexList:
		var idxCmd tea.Cmd
		m.idxList, idxCmd = m.idxList.Update(msg)
		if idxCmd != nil {
			return m.dispatch(idxCmd())
		}
	case viewDelete:
		var delCmd tea.Cmd
		m.delete, delCmd = m.delete.Update(msg)
		if delCmd != nil {
			return m.dispatch(delCmd())
		}
	case viewConfirmWrite:
		var cwCmd tea.Cmd
		m.confirmWrite, cwCmd = m.confirmWrite.Update(msg)
		if cwCmd != nil {
			result, ok := cwCmd().(confirmResultMsg)
			if !ok {
				return m, cwCmd
			}
			m = m.popView()
			if !result.Confirmed {
				m.pendingDoc = nil
				m.pendingIndexKeysJSON = ""
				return m, nil
			}
			return m.executePendingWrite()
		}
	}
	return m, cmd
}

// executePendingWrite performs the write that was just confirmed via
// viewConfirmWrite: either a document insert/replace (set up by
// insertRequestedMsg/editFullRequestedMsg) or an index creation (set up by
// indexCreateSubmittedMsg).
func (m RootModel) executePendingWrite() (tea.Model, tea.Cmd) {
	client, db, coll := m.client, m.db, m.coll

	if m.pendingIndexKeysJSON != "" {
		var keys bson.D
		if err := bson.UnmarshalExtJSON([]byte(m.pendingIndexKeysJSON), false, &keys); err != nil {
			m.err = fmt.Errorf("keys de índice inválidas: %w", err)
			return m, nil
		}
		unique := m.pendingIndexUnique
		m.pendingIndexKeysJSON = ""
		return m, func() tea.Msg {
			_, err := client.CreateIndex(context.Background(), db, coll, keys, unique)
			return indexWriteCompletedMsg{Err: err}
		}
	}

	doc, mode := m.pendingDoc, m.editMode
	switch mode {
	case "insert":
		return m, func() tea.Msg {
			_, err := client.InsertOne(context.Background(), db, coll, doc)
			return docWriteCompletedMsg{Err: err}
		}
	case "edit":
		id := m.docDetail.doc["_id"]
		return m, func() tea.Msg {
			err := client.ReplaceOne(context.Background(), db, coll, id, doc)
			return docWriteCompletedMsg{Err: err}
		}
	}
	return m, nil
}

// dispatch re-enters Update with an already-produced message, used by
// sub-models whose Update signature returns a concrete child type instead
// of tea.Model.
func (m RootModel) dispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.Update(msg)
}

// statusBar renders the current connection's name in its assigned color, so
// it's always visible which environment (e.g. qa vs. prod) is being edited.
func (m RootModel) statusBar() string {
	if m.conn.Name == "" {
		return ""
	}
	label := fmt.Sprintf(" %s ", m.conn.Name)
	return colorStyle(m.conn.Color).Reverse(true).Render(label) + "\n\n"
}

func (m RootModel) View() string {
	if m.err != nil {
		return m.statusBar() + fmt.Sprintf("Error: %v\n\n[cualquier tecla] continuar", m.err)
	}

	var content string
	switch m.view {
	case viewConnectionPicker:
		return m.connPicker.View() // no connection chosen yet, no status bar
	case viewDatabaseList:
		content = m.dbList.View()
	case viewCollectionList:
		content = m.collList.View()
	case viewDocumentList:
		content = m.docList.View()
	case viewDocumentDetail:
		content = m.docDetail.View()
	case viewFieldEdit:
		content = m.fieldEdit.View()
	case viewIndexList:
		content = m.idxList.View()
	case viewDelete:
		content = m.delete.View()
	case viewConfirmWrite:
		content = m.confirmWrite.View()
	case viewHelp:
		content = helpModel{}.View()
	}
	return m.statusBar() + content
}

// Run starts the Bubbletea program.
func Run(client mongo.Client, resolved *config.Connection) error {
	model := NewRootModel(client, resolved)
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v -run TestRootModel`
Expected: PASS

- [ ] **Step 6: Wire up main.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"github.com/jonathanleivag/lazymongo/internal/tui"
)

func main() {
	client := mongo.NewRealClient()

	if len(os.Args) < 2 {
		if err := tui.Run(client, nil); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	conn, err := config.ResolveConnection(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if err := tui.Run(client, &conn); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 7: Build and run the full test suite**

Run:
```bash
go build -o lazymongo .
go test ./...
go vet ./...
```
Expected: builds cleanly, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/help.go internal/tui/root.go internal/tui/root_test.go main.go
git commit -m "feat: wire root model, help overlay, and main entry point"
```

---

### Task 17: README usage docs and manual smoke test against `qa`

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: everything built in Tasks 1–16
- Produces: a documented, manually-verified working binary

- [ ] **Step 1: Expand README.md with full usage instructions**

```markdown
# lazymongo

Personal TUI for browsing/editing MongoDB, reusing `~/.config/mongo-connections.sh`
(the same file the `mgo` shell function uses). See
`docs/superpowers/specs/2026-07-11-lazymongo-design.md` for the design and
`docs/superpowers/plans/2026-07-11-lazymongo.md` for how it was built.

## Build

    go build -o lazymongo .

## Run

    ./lazymongo <connection-name>   # e.g. ./lazymongo qa
    ./lazymongo                     # shows a picker of available connections

## Keybindings

| Key | Action |
|---|---|
| `j`/`k`, arrows | move |
| `Enter` | drill down / select |
| `Esc`, `h` | go back |
| `/` | filter documents |
| `Tab` | switch Documents/Indexes |
| `n`/`p` | next/previous page |
| `i`, `a` | insert document / create connection or index |
| `e` | edit field inline |
| `E` | edit full document in `$EDITOR` |
| `d`, `x` | delete (always confirms) |
| `?` | help |
| `q`, `Ctrl+c` | quit |

## Testing

    go test ./...                    # unit tests, no Docker required
    ./scripts/test-integration.sh    # integration tests against a disposable local Mongo (requires Docker Desktop running)

Integration tests never run against `qa`/`prod` — only a throwaway
`mongo:7` container on port 27018.
```

- [ ] **Step 2: Manual smoke test against `qa` (do this by hand, not automated)**

Run: `./lazymongo qa`

Walk through, confirming each works as expected:
- [ ] Databases list loads
- [ ] Drilling into a collection loads documents
- [ ] `/` filter with a real JSON filter narrows results
- [ ] `n`/`p` paginate
- [ ] Opening a document (`Enter`) shows its fields
- [ ] `e` on a field shows the confirmation prompt, and confirming actually updates the field (verify with `mongosh`/`mgo qa` afterward)
- [ ] `E` opens the full document in `nvim`, and saving+exiting replaces it
- [ ] `i` inserts a new document after confirmation
- [ ] `d` on a document asks to confirm before deleting
- [ ] `Tab` switches to the index list; `a` creates an index, `d` drops one (both confirm first)
- [ ] The status bar/color reflects `qa`'s assigned color once one is set in `mongo-connections.sh`
- [ ] `?` shows help; `q`/`Ctrl+c` quits cleanly

Only after this passes should `prod` be used with `lazymongo`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: expand README with usage, keybindings, and testing instructions"
```
