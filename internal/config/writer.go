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
//
// conn.Name is validated with isValidConnectionName before anything is
// written. This is a security boundary, not just a formatting nicety:
// insertConnection interpolates conn.Name RAW (unescaped) into zsh
// array-subscript syntax ("[%s]="), while conn.URI and conn.Color are
// escaped via %q. An unvalidated name such as `x]="y"; rm -rf ~ #` would
// corrupt the array syntax and inject arbitrary shell commands into
// ~/.config/mongo-connections.sh — a file sourced automatically by .zshrc
// every time a new terminal opens, on this machine or anyone else's who
// reuses this dotfile. That makes it a stored/persistent injection, worse
// than a one-off command injection: the payload survives and re-executes
// on every future shell startup, not just the run that planted it.
func AddConnection(conn Connection) error {
	if !isValidConnectionName(conn.Name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", conn.Name)
	}

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
	content, err := insertIntoArray(content, "MONGO_CONNECTIONS", fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.URI)))
	if err != nil {
		return "", err
	}
	content, err = insertIntoArray(content, "MONGO_CONNECTION_COLORS", fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.Color)))
	if err != nil {
		return "", err
	}
	return content, nil
}

// zshSingleQuote wraps s in zsh single quotes, safe for literal
// interpolation as an array *value* (conn.URI/conn.Color — conn.Name is a
// separate case, validated by isValidConnectionName and never quoted, since
// it's the array subscript itself). This replaces the previous
// fmt.Sprintf("%q", s): Go's %q escapes for GO's own double-quote syntax,
// not zsh's — a zsh double-quoted string still performs $(...)/backtick
// command substitution and backslash escapes, so a value like
// "mongodb://x$(cmd)y" wrote a live command substitution into
// ~/.config/mongo-connections.sh, a file .zshrc sources on every new
// terminal (confirmed via a real exploit reproduction during final
// review — a persistent/stored RCE, not a one-off). zsh single-quoted
// strings perform no expansion at all; the only character that needs
// handling inside one is a literal single quote, escaped by closing the
// quote, emitting an escaped literal quote, and reopening: '\''.
func zshSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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

// UpdateConnection replaces an existing connection's URI and color in the
// real connections file, keeping its name (the array key) unchanged.
// Mirrors AddConnection's safety model: validates the result is still
// valid zsh before keeping the change, restoring the original file on any
// failure.
func UpdateConnection(conn Connection) error {
	if !isValidConnectionName(conn.Name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", conn.Name)
	}

	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}

	updated, err := updateConnectionInFile(string(original), conn)
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

func updateConnectionInFile(content string, conn Connection) (string, error) {
	content, err := replaceOrInsertInArray(content, "MONGO_CONNECTIONS", conn.Name, fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.URI)))
	if err != nil {
		return "", err
	}
	content, err = replaceOrInsertInArray(content, "MONGO_CONNECTION_COLORS", conn.Name, fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.Color)))
	if err != nil {
		return "", err
	}
	return content, nil
}

// replaceOrInsertInArray replaces the existing "[name]=..." line within the
// named zsh associative array with newLine, or inserts it (same fallback as
// insertIntoArray) if the array exists but doesn't yet have an entry for
// name — creating the array block fresh if it doesn't exist at all. Used by
// UpdateConnection; insertIntoArray (used by AddConnection) is untouched —
// this is a separate function, not a refactor of already-shipped behavior.
func replaceOrInsertInArray(content, arrayName, name, newLine string) (string, error) {
	header := fmt.Sprintf("declare -A %s=(", arrayName)
	keyPrefix := fmt.Sprintf("[%s]=", name)

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
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ")" {
			result := make([]string, 0, len(lines)+1)
			result = append(result, lines[:i]...)
			result = append(result, newLine)
			result = append(result, lines[i:]...)
			return strings.Join(result, "\n"), nil
		}
		if strings.HasPrefix(trimmed, keyPrefix) {
			result := make([]string, len(lines))
			copy(result, lines)
			result[i] = newLine
			return strings.Join(result, "\n"), nil
		}
	}
	return "", fmt.Errorf("no se encontró el cierre ')' del array %s", arrayName)
}

// DeleteConnection removes a connection from the real connections file.
// Mirrors AddConnection's safety model. Removing from
// MONGO_CONNECTION_COLORS is a no-op (not an error) when the connection
// never had a color entry.
func DeleteConnection(name string) error {
	if !isValidConnectionName(name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", name)
	}

	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}

	updated, err := removeFromArray(string(original), "MONGO_CONNECTIONS", name)
	if err != nil {
		return err
	}
	updated, err = removeFromArray(updated, "MONGO_CONNECTION_COLORS", name)
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

// removeFromArray removes the "[name]=..." line from the named zsh
// associative array, or does nothing (returns content unchanged) if the
// array doesn't exist or has no entry for name.
func removeFromArray(content, arrayName, name string) (string, error) {
	header := fmt.Sprintf("declare -A %s=(", arrayName)
	if !strings.Contains(content, header) {
		return content, nil
	}

	keyPrefix := fmt.Sprintf("[%s]=", name)
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
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ")" {
			return content, nil
		}
		if strings.HasPrefix(trimmed, keyPrefix) {
			result := make([]string, 0, len(lines)-1)
			result = append(result, lines[:i]...)
			result = append(result, lines[i+1:]...)
			return strings.Join(result, "\n"), nil
		}
	}
	return content, nil
}

// arrayHasKey reports whether the named zsh associative array already
// contains an entry for name. Used by RenameConnection to detect a
// collision before writing anything — renaming into an existing different
// connection's name would otherwise silently overwrite it.
func arrayHasKey(content, arrayName, name string) bool {
	header := fmt.Sprintf("declare -A %s=(", arrayName)
	if !strings.Contains(content, header) {
		return false
	}

	keyPrefix := fmt.Sprintf("[%s]=", name)
	lines := strings.Split(content, "\n")
	headerLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			headerLineIdx = i
			break
		}
	}
	if headerLineIdx == -1 {
		return false
	}

	for i := headerLineIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ")" {
			return false
		}
		if strings.HasPrefix(trimmed, keyPrefix) {
			return true
		}
	}
	return false
}

// RenameConnection moves an existing connection from oldName to conn.Name,
// updating both MONGO_CONNECTIONS and MONGO_CONNECTION_COLORS. If conn.Name
// differs from oldName and already names a different existing connection,
// the rename is rejected (no file is written) to avoid silently
// overwriting that connection. Mirrors AddConnection/UpdateConnection's
// safety model: validates the result is still valid zsh before keeping the
// change, restoring the original file on any failure.
func RenameConnection(oldName string, conn Connection) error {
	if !isValidConnectionName(oldName) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", oldName)
	}
	if !isValidConnectionName(conn.Name) {
		return fmt.Errorf("nombre de conexión inválido %q: solo se permiten letras, números, guiones y guiones bajos", conn.Name)
	}

	original, err := os.ReadFile(connectionsFile)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", connectionsFile, err)
	}
	content := string(original)

	if conn.Name != oldName && arrayHasKey(content, "MONGO_CONNECTIONS", conn.Name) {
		return fmt.Errorf("ya existe una conexión llamada %q", conn.Name)
	}

	content, err = removeFromArray(content, "MONGO_CONNECTIONS", oldName)
	if err != nil {
		return err
	}
	content, err = removeFromArray(content, "MONGO_CONNECTION_COLORS", oldName)
	if err != nil {
		return err
	}
	content, err = insertIntoArray(content, "MONGO_CONNECTIONS", fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.URI)))
	if err != nil {
		return err
	}
	content, err = insertIntoArray(content, "MONGO_CONNECTION_COLORS", fmt.Sprintf("  [%s]=%s", conn.Name, zshSingleQuote(conn.Color)))
	if err != nil {
		return err
	}

	if err := os.WriteFile(connectionsFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("escribiendo %s: %w", connectionsFile, err)
	}

	if err := validateZshSyntax(connectionsFile); err != nil {
		_ = os.WriteFile(connectionsFile, original, 0600)
		return fmt.Errorf("el archivo resultante no era zsh válido, se revirtió: %w", err)
	}
	return nil
}
