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
