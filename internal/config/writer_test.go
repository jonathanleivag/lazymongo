package config

import (
	"fmt"
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

// TestAddConnection_RejectsUnsafeName proves the fix for the stored/persistent
// shell-injection bug: an unsafe conn.Name must never reach insertConnection,
// because it is interpolated raw (no escaping) into zsh array-subscript
// syntax when writing the file. If it were written, sourcing the file later
// (which .zshrc does on every new terminal) would execute arbitrary shell
// commands. AddConnection must reject the name AND leave the file byte-for-
// byte unchanged.
func TestAddConnection_RejectsUnsafeName(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before AddConnection: %v", err)
	}

	malicious := `x]="y"; rm -rf ~ #`
	err = AddConnection(Connection{Name: malicious, URI: "mongodb://x", Color: "verde"})
	if err == nil {
		t.Fatal("expected an error for an unsafe connection name, got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after AddConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected name:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	if _, resolveErr := ResolveConnection("qa"); resolveErr != nil {
		t.Fatalf("existing connection broke after rejected AddConnection: %v", resolveErr)
	}
}

func TestUpdateConnection_ChangesURIAndColorKeepingName(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	err := UpdateConnection(Connection{Name: "qa", URI: "mongodb://newhost:27017/qa2", Color: "rojo"})
	if err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("resolving updated connection: %v", err)
	}
	want := Connection{Name: "qa", URI: "mongodb://newhost:27017/qa2", Color: "rojo"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}

	other, err := ResolveConnection("prod")
	if err != nil {
		t.Fatalf("resolving untouched connection: %v", err)
	}
	if other.URI != "mongodb://localhost:27017/prod" || other.Color != "rojo" {
		t.Fatalf("untouched connection changed unexpectedly: %+v", other)
	}
}

func TestUpdateConnection_WorksWhenConnectionNeverHadAColor(t *testing.T) {
	withTempConnectionsFile(t, "testdata/with_comments.sh")

	err := UpdateConnection(Connection{Name: "ejemplo-local", URI: "mongodb://localhost:27017/renamed", Color: "verde"})
	if err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}

	conn, err := ResolveConnection("ejemplo-local")
	if err != nil {
		t.Fatalf("resolving updated connection: %v", err)
	}
	want := Connection{Name: "ejemplo-local", URI: "mongodb://localhost:27017/renamed", Color: "verde"}
	if conn != want {
		t.Fatalf("got %+v, want %+v", conn, want)
	}
}

func TestUpdateConnection_ResultIsValidZsh(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := UpdateConnection(Connection{Name: "qa", URI: "mongodb://x", Color: "amarillo"}); err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}

func TestUpdateConnection_RejectsUnsafeName(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before UpdateConnection: %v", err)
	}

	malicious := `x]="y"; rm -rf ~ #`
	err = UpdateConnection(Connection{Name: malicious, URI: "mongodb://x", Color: "verde"})
	if err == nil {
		t.Fatal("expected an error for an unsafe connection name, got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after UpdateConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected name")
	}
}

func TestDeleteConnection_RemovesConnectionFromBothArrays(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	if err := DeleteConnection("qa"); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	if _, err := ResolveConnection("qa"); err == nil {
		t.Fatal("expected 'qa' to no longer resolve after deletion")
	}

	other, err := ResolveConnection("prod")
	if err != nil {
		t.Fatalf("resolving untouched connection: %v", err)
	}
	if other.URI != "mongodb://localhost:27017/prod" || other.Color != "rojo" {
		t.Fatalf("untouched connection changed unexpectedly: %+v", other)
	}
}

func TestDeleteConnection_NoOpWhenConnectionNeverHadAColor(t *testing.T) {
	withTempConnectionsFile(t, "testdata/with_comments.sh")

	if err := DeleteConnection("ejemplo-local"); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	if _, err := ResolveConnection("ejemplo-local"); err == nil {
		t.Fatal("expected 'ejemplo-local' to no longer resolve after deletion")
	}
}

func TestDeleteConnection_ResultIsValidZsh(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := DeleteConnection("qa"); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}

func TestDeleteConnection_RejectsUnsafeName(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file before DeleteConnection: %v", err)
	}

	malicious := `x]="y"; rm -rf ~ #`
	err = DeleteConnection(malicious)
	if err == nil {
		t.Fatal("expected an error for an unsafe connection name, got nil")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file after DeleteConnection: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("file was modified despite rejected name")
	}
}

// TestAddConnection_NeutralizesShellMetacharactersInURI proves the fix for a
// second shell-injection vector found in final review: unlike conn.Name
// (validated and rejected outright if unsafe), conn.URI/conn.Color were
// never validated — they went straight into the zsh array value via
// fmt.Sprintf("%q", ...). Go's %q escapes for GO's own double-quote syntax,
// not zsh's: a zsh double-quoted string still performs $(...)/backtick
// command substitution, so a URI like "mongodb://x$(cmd)y" wrote a live
// command substitution into ~/.config/mongo-connections.sh — a file
// .zshrc sources on every new terminal, making this a persistent/stored
// RCE, not a one-off. The fix wraps the value in zsh single quotes instead
// (see zshSingleQuote), which perform no expansion at all.
func TestAddConnection_NeutralizesShellMetacharactersInURI(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	marker := filepath.Join(t.TempDir(), "pwned")
	malicious := fmt.Sprintf("mongodb://x$(touch %s)y", marker)

	if err := AddConnection(Connection{Name: "evil", URI: malicious, Color: "verde"}); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command substitution in URI was executed — shell injection not neutralized")
	}

	conn, err := ResolveConnection("evil")
	if err != nil {
		t.Fatalf("resolving connection: %v", err)
	}
	if conn.URI != malicious {
		t.Fatalf("expected URI preserved literally as %q, got %q", malicious, conn.URI)
	}
}

func TestAddConnection_PreservesSingleQuoteInURI(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	uri := "mongodb://user:p'ass@host/db"
	if err := AddConnection(Connection{Name: "quoted", URI: uri, Color: "rojo"}); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	conn, err := ResolveConnection("quoted")
	if err != nil {
		t.Fatalf("resolving connection: %v", err)
	}
	if conn.URI != uri {
		t.Fatalf("expected URI %q preserved intact, got %q", uri, conn.URI)
	}
}

func TestAddConnection_ResultIsValidZshWithMetacharactersInURI(t *testing.T) {
	path := withTempConnectionsFile(t, "testdata/basic.sh")

	if err := AddConnection(Connection{Name: "meta", URI: "a`b$c\\d'e", Color: "amarillo"}); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}
	if err := validateZshSyntax(path); err != nil {
		t.Fatalf("resulting file is not valid zsh: %v", err)
	}
}

func TestUpdateConnection_NeutralizesShellMetacharactersInURI(t *testing.T) {
	withTempConnectionsFile(t, "testdata/basic.sh")

	marker := filepath.Join(t.TempDir(), "pwned")
	malicious := fmt.Sprintf("mongodb://x$(touch %s)y", marker)

	if err := UpdateConnection(Connection{Name: "qa", URI: malicious, Color: "verde"}); err != nil {
		t.Fatalf("UpdateConnection failed: %v", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command substitution in URI was executed — shell injection not neutralized")
	}

	conn, err := ResolveConnection("qa")
	if err != nil {
		t.Fatalf("resolving connection: %v", err)
	}
	if conn.URI != malicious {
		t.Fatalf("expected URI preserved literally as %q, got %q", malicious, conn.URI)
	}
}
