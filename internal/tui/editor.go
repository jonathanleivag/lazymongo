package tui

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
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
