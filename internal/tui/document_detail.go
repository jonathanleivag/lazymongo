package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fieldSelectedMsg struct {
	Field string
	Value any
}
type editFullRequestedMsg struct{}
type deleteRequestedMsg struct{}
type valueCopiedMsg struct {
	Text string
}

type treeNode struct {
	key        string
	value      any
	level      int
	isParent   bool
	isExpanded bool
	children   []*treeNode
	parent     *treeNode
}

func (n *treeNode) fullPath() string {
	if n.parent == nil || n.parent.key == "" {
		return n.key
	}
	parentPath := n.parent.fullPath()
	if parentPath == "" {
		return n.key
	}
	return parentPath + "." + n.key
}

func buildTree(key string, v any, level int, parent *treeNode) *treeNode {
	node := &treeNode{
		key:        key,
		value:      v,
		level:      level,
		isExpanded: false, // Collapse by default so the user can choose to expand them!
		parent:     parent,
	}
	if mapVal, ok := toMap(v); ok {
		node.isParent = true
		node.value = mapVal // Normalize value to bson.M for rendering/copying!
		var keys []string
		for k := range mapVal {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			node.children = append(node.children, buildTree(k, mapVal[k], level+1, node))
		}
	} else if arrVal, ok := toArray(v); ok {
		node.isParent = true
		node.value = arrVal // Normalize value to []any for rendering/copying!
		for idx, item := range arrVal {
			node.children = append(node.children, buildTree(fmt.Sprintf("%d", idx), item, level+1, node))
		}
	}
	return node
}

func gatherVisibleNodes(node *treeNode, list []*treeNode) []*treeNode {
	if node.key != "" || node.level >= 0 {
		list = append(list, node)
	}
	if (node.key == "" && node.level == -1) || (node.isParent && node.isExpanded) {
		for _, child := range node.children {
			list = gatherVisibleNodes(child, list)
		}
	}
	return list
}

type docDetailModel struct {
	doc          bson.M
	root         *treeNode
	visibleNodes []*treeNode
	cursor       int
}

func newDocDetailModel(doc bson.M) docDetailModel {
	dummyRoot := &treeNode{
		key:        "",
		value:      doc,
		level:      -1,
		isParent:   true,
		isExpanded: true,
	}
	var keys []string
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		dummyRoot.children = append(dummyRoot.children, buildTree(k, doc[k], 0, dummyRoot))
	}

	m := docDetailModel{doc: doc, root: dummyRoot}
	m.visibleNodes = gatherVisibleNodes(m.root, nil)
	return m
}

func (m docDetailModel) Update(msg tea.Msg) (docDetailModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if m.cursor < len(m.visibleNodes)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter", "space":
		if len(m.visibleNodes) > 0 {
			node := m.visibleNodes[m.cursor]
			if node.isParent {
				node.isExpanded = !node.isExpanded
				m.visibleNodes = gatherVisibleNodes(m.root, nil)
				if m.cursor >= len(m.visibleNodes) {
					m.cursor = len(m.visibleNodes) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		}
	case "e":
		if len(m.visibleNodes) > 0 {
			node := m.visibleNodes[m.cursor]
			if !node.isParent {
				return m, func() tea.Msg { return fieldSelectedMsg{Field: node.fullPath(), Value: node.value} }
			}
		}
	case "y", "c":
		if len(m.visibleNodes) > 0 {
			node := m.visibleNodes[m.cursor]
			text := valueToCopyString(node.value)
			_ = copyToClipboard(text)
			return m, func() tea.Msg { return valueCopiedMsg{Text: text} }
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
	for i, node := range m.visibleNodes {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("> ")
		}
		indent := strings.Repeat("  ", node.level)
		
		var line string
		if node.isParent {
			indicator := "▶ "
			if node.isExpanded {
				indicator = "▼ "
			}
			var typeStr string
			switch val := node.value.(type) {
			case bson.M:
				typeStr = "Object"
			case []any:
				n := len(val)
				if n > 0 {
					typeStr = fmt.Sprintf("Array (%d)", n)
				} else {
					typeStr = "Array (empty)"
				}
			}
			line = fmt.Sprintf("%s%s%s: %s", indent, indicator, node.key, helpHintStyle.Render(typeStr))
		} else {
			line = fmt.Sprintf("%s  %s: %s", indent, node.key, styleBSONValue(node.value))
		}
		
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString("\n" + helpHintStyle.Render("[Enter/Espacio] expandir/colapsar  [e] editar campo  [y] copiar valor  [E] editar completo  [d] borrar  [Esc] volver"))
	return b.String()
}

func valueToCopyString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case bson.ObjectID:
		return val.Hex()
	case string:
		return val
	case int32, int64, int, float64, float32, bool:
		return fmt.Sprintf("%v", val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		if bytes, err := bson.MarshalExtJSON(val, true, false); err == nil {
			return string(bytes)
		}
		return fmt.Sprintf("%v", val)
	}
}

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default: // linux, freebsd, etc.
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or xsel)")
		}
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := in.Write([]byte(text)); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
