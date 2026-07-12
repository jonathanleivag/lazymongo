package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// validConnectionNamePattern restricts connection names to characters that
// are safe to interpolate into a zsh array-subscript expression. This is a
// security boundary: names come from user input (typed directly today, and
// eventually via a "create connection" TUI form), and the zsh script that
// resolves a connection is built with fmt.Sprintf and run via
// exec.Command("zsh", "-c", script). Anything outside this charset could
// break out of the ${MONGO_CONNECTIONS[%s]} subscript and inject arbitrary
// shell commands.
var validConnectionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// isValidConnectionName reports whether name is safe to interpolate into the
// zsh script built by ResolveConnection/ListConnections. It must be called
// before any such interpolation happens.
func isValidConnectionName(name string) bool {
	return validConnectionNamePattern.MatchString(name)
}

// SetConnectionsFileForTesting points the connections file at a fixture path for tests
// and returns a restore function to call via t.Cleanup. Not for production use.
func SetConnectionsFileForTesting(path string) (restore func()) {
	old := connectionsFile
	connectionsFile = path
	return func() { connectionsFile = old }
}

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
	if !isValidConnectionName(name) {
		return Connection{}, fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", name)
	}

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
