package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.mongodb.org/mongo-driver/v2/bson"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
)

type metricsPolledMsg struct {
	Data *metricsData
	Err  error
}

type ActiveOpInfo struct {
	OpID      int64
	Namespace string
	Duration  string
	Op        string
}

type metricsData struct {
	Time        time.Time
	MemVirtual  float64
	MemRes      float64
	ConnCurrent int
	ConnAvail   int
	NetBytesIn  int64
	NetBytesOut int64

	OpsInsert  int64
	OpsQuery   int64
	OpsUpdate  int64
	OpsDelete  int64
	OpsGetMore int64
	OpsCommand int64

	InsertRate  float64
	QueryRate   float64
	UpdateRate  float64
	DeleteRate  float64
	GetMoreRate float64
	CommandRate float64
	NetInRate   float64
	NetOutRate  float64

	ActiveOps []ActiveOpInfo
}

type metricsModel struct {
	client mongo.Client
	data   *metricsData
	err    error
}

func newMetricsModel(client mongo.Client) metricsModel {
	return metricsModel{client: client}
}

func (m metricsModel) initCmd() tea.Cmd {
	return pollMetricsCmd(m.client, nil, 0)
}

func (m metricsModel) Update(msg tea.Msg) (metricsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "m":
			return m, func() tea.Msg { return listBackMsg{} }
		}
	case metricsPolledMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, pollMetricsCmd(m.client, m.data, 1*time.Second)
		}
		m.data = msg.Data
		m.err = nil
		// Continue polling in background
		return m, pollMetricsCmd(m.client, m.data, 1*time.Second)
	}
	return m, nil
}

func (m metricsModel) View(width, height int) string {
	if m.err != nil && m.data == nil {
		var errMsg strings.Builder
		errMsg.WriteString(fmt.Sprintf("Error cargando métricas: %v\n\n", m.err))
		errMsg.WriteString("Reintentando conexión automáticamente en el fondo...\n\n")
		errMsg.WriteString(helpHintStyle.Render("[Esc/q] Volver a lazymongo"))
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("9")).
			Padding(1, 2).
			Render(errMsg.String())
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}
	if m.data == nil {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, "Cargando métricas de rendimiento...")
	}

	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(focusedBorderColor)
	if m.err != nil {
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
		b.WriteString(headerStyle.Render("=== MONITOREO DE RENDIMIENTO DE MONGODB ===") + "  " + warnStyle.Render("[⚠️ ERROR DE CONEXIÓN: Reintentando...]") + "\n\n")
	} else {
		b.WriteString(headerStyle.Render("=== MONITOREO DE RENDIMIENTO DE MONGODB ===") + "\n\n")
	}

	// Memory & Connections row
	memTitle := lipgloss.NewStyle().Bold(true).Render("MEMORIA")
	connTitle := lipgloss.NewStyle().Bold(true).Render("CONEXIONES")

	memMax := m.data.MemVirtual
	if memMax < 16.0 {
		memMax = 16.0
	}
	memBar := renderProgressBar(m.data.MemRes, memMax, 20, "2", "8")

	connMax := float64(m.data.ConnCurrent + m.data.ConnAvail)
	connBar := renderProgressBar(float64(m.data.ConnCurrent), connMax, 20, "6", "8")

	b.WriteString(fmt.Sprintf("%-40s %-40s\n", memTitle, connTitle))
	b.WriteString(fmt.Sprintf("Virtual:   %-31.2f GB   Activas:     %-31d\n", m.data.MemVirtual, m.data.ConnCurrent))
	b.WriteString(fmt.Sprintf("Residente: %-31.2f GB   Disponibles: %-31d\n", m.data.MemRes, m.data.ConnAvail))
	b.WriteString(fmt.Sprintf("Uso:       %-31s   Uso:         %-31s\n\n", memBar, connBar))

	// Operations section
	opsTitle := lipgloss.NewStyle().Bold(true).Render("OPERACIONES POR SEGUNDO")
	b.WriteString(opsTitle + "\n")
	b.WriteString(fmt.Sprintf("  Insertar:    %6.1f ops/s   |   Consulta:  %6.1f ops/s\n", m.data.InsertRate, m.data.QueryRate))
	b.WriteString(fmt.Sprintf("  Actualizar:  %6.1f ops/s   |   Borrar:    %6.1f ops/s\n", m.data.UpdateRate, m.data.DeleteRate))
	b.WriteString(fmt.Sprintf("  Comando:     %6.1f ops/s   |   GetMore:   %6.1f ops/s\n\n", m.data.CommandRate, m.data.GetMoreRate))

	// Network section
	netTitle := lipgloss.NewStyle().Bold(true).Render("RENDIMIENTO DE RED")
	b.WriteString(netTitle + "\n")
	b.WriteString(fmt.Sprintf("  Entrada:     %-30s\n", formatBytesRate(m.data.NetInRate)))
	b.WriteString(fmt.Sprintf("  Salida:      %-30s\n\n", formatBytesRate(m.data.NetOutRate)))

	// Slowest / Active Operations section
	opsActiveTitle := lipgloss.NewStyle().Bold(true).Render("OPERACIONES ACTIVAS (currentOp)")
	b.WriteString(opsActiveTitle + "\n")
	if len(m.data.ActiveOps) == 0 {
		b.WriteString("  (ninguna operación activa en ejecución)\n")
	} else {
		b.WriteString(fmt.Sprintf("  %-10s %-40s %-12s %-10s\n", "OPID", "COLECCIÓN", "DURACIÓN", "TIPO"))
		for _, op := range m.data.ActiveOps {
			ns := op.Namespace
			if len(ns) > 40 {
				ns = ns[:37] + "..."
			}
			b.WriteString(fmt.Sprintf("  %-10d %-40s %-12s %-10s\n", op.OpID, ns, op.Duration, strings.ToUpper(op.Op)))
		}
	}

	b.WriteString("\n" + helpHintStyle.Render("[Esc/q] Volver a lazymongo"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusedBorderColor).
		Padding(1, 2).
		Width(width - 6).
		Height(height - 4).
		Render(b.String())

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func renderProgressBar(current, max float64, width int, barColor, emptyColor string) string {
	if max <= 0 {
		return strings.Repeat("░", width)
	}
	ratio := current / max
	if ratio > 1.0 {
		ratio = 1.0
	}
	filledWidth := int(ratio * float64(width))
	if filledWidth < 0 {
		filledWidth = 0
	}
	emptyWidth := width - filledWidth

	filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(emptyColor))

	return filledStyle.Render(strings.Repeat("█", filledWidth)) + emptyStyle.Render(strings.Repeat("░", emptyWidth))
}

func formatBytesRate(rate float64) string {
	if rate < 1024 {
		return fmt.Sprintf("%.1f B/s", rate)
	} else if rate < 1024*1024 {
		return fmt.Sprintf("%.2f KB/s", rate/1024)
	} else {
		return fmt.Sprintf("%.2f MB/s", rate/(1024*1024))
	}
}

func pollMetricsCmd(client mongo.Client, prev *metricsData, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		if delay > 0 {
			time.Sleep(delay)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		statusRaw, err := client.RunAdminCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}})
		if err != nil {
			return metricsPolledMsg{Err: err}
		}

		var activeOps []ActiveOpInfo
		opRaw, err := client.RunAdminCommand(ctx, bson.D{
			{Key: "currentOp", Value: 1},
			{Key: "active", Value: true},
		})
		if err == nil {
			if inprog, ok := opRaw["inprog"].(bson.A); ok {
				for _, opVal := range inprog {
					if opMap, ok := opVal.(bson.M); ok {
						opid := int64(0)
						if idVal, ok := opMap["opid"]; ok {
							switch v := idVal.(type) {
							case int32:
								opid = int64(v)
							case int64:
								opid = v
							}
						}
						ns := ""
						if nsVal, ok := opMap["ns"].(string); ok {
							ns = nsVal
						}
						opType := ""
						if opVal, ok := opMap["op"].(string); ok {
							opType = opVal
						}
						durationUs := int64(0)
						if durVal, ok := opMap["microsecs_running"]; ok {
							switch v := durVal.(type) {
							case int32:
								durationUs = int64(v)
							case int64:
								durationUs = v
							}
						}
						durationStr := fmt.Sprintf("%.2f ms", float64(durationUs)/1000.0)
						if durationUs > 1000000 {
							durationStr = fmt.Sprintf("%.2f s", float64(durationUs)/1000000.0)
						}
						// Only list ops with positive duration or defined types
						if opid > 0 && ns != "" {
							activeOps = append(activeOps, ActiveOpInfo{
								OpID:      opid,
								Namespace: ns,
								Duration:  durationStr,
								Op:        opType,
							})
						}
					}
				}
			}
		}

		data := parseServerStatus(statusRaw, prev)
		data.ActiveOps = activeOps
		return metricsPolledMsg{Data: data}
	}
}

func parseServerStatus(status bson.M, prev *metricsData) *metricsData {
	data := &metricsData{
		Time: time.Now(),
	}

	if mem, ok := status["mem"].(bson.M); ok {
		if v, ok := mem["virtual"]; ok {
			switch val := v.(type) {
			case int32:
				data.MemVirtual = float64(val) / 1024.0
			case int64:
				data.MemVirtual = float64(val) / 1024.0
			case float64:
				data.MemVirtual = val / 1024.0
			}
		}
		if r, ok := mem["resident"]; ok {
			switch val := r.(type) {
			case int32:
				data.MemRes = float64(val) / 1024.0
			case int64:
				data.MemRes = float64(val) / 1024.0
			case float64:
				data.MemRes = val / 1024.0
			}
		}
	}

	if conns, ok := status["connections"].(bson.M); ok {
		if c, ok := conns["current"]; ok {
			switch val := c.(type) {
			case int32:
				data.ConnCurrent = int(val)
			case int64:
				data.ConnCurrent = int(val)
			}
		}
		if a, ok := conns["available"]; ok {
			switch val := a.(type) {
			case int32:
				data.ConnAvail = int(val)
			case int64:
				data.ConnAvail = int(val)
			}
		}
	}

	if net, ok := status["network"].(bson.M); ok {
		if bi, ok := net["bytesIn"]; ok {
			switch val := bi.(type) {
			case int32:
				data.NetBytesIn = int64(val)
			case int64:
				data.NetBytesIn = val
			}
		}
		if bo, ok := net["bytesOut"]; ok {
			switch val := bo.(type) {
			case int32:
				data.NetBytesOut = int64(val)
			case int64:
				data.NetBytesOut = val
			}
		}
	}

	if ops, ok := status["opcounters"].(bson.M); ok {
		if ins, ok := ops["insert"]; ok {
			data.OpsInsert = toInt64(ins)
		}
		if q, ok := ops["query"]; ok {
			data.OpsQuery = toInt64(q)
		}
		if u, ok := ops["update"]; ok {
			data.OpsUpdate = toInt64(u)
		}
		if d, ok := ops["delete"]; ok {
			data.OpsDelete = toInt64(d)
		}
		if gm, ok := ops["getmore"]; ok {
			data.OpsGetMore = toInt64(gm)
		}
		if cmd, ok := ops["command"]; ok {
			data.OpsCommand = toInt64(cmd)
		}
	}

	if prev != nil {
		delta := data.Time.Sub(prev.Time).Seconds()
		if delta > 0 {
			data.InsertRate = float64(data.OpsInsert-prev.OpsInsert) / delta
			data.QueryRate = float64(data.OpsQuery-prev.OpsQuery) / delta
			data.UpdateRate = float64(data.OpsUpdate-prev.OpsUpdate) / delta
			data.DeleteRate = float64(data.OpsDelete-prev.OpsDelete) / delta
			data.GetMoreRate = float64(data.OpsGetMore-prev.OpsGetMore) / delta
			data.CommandRate = float64(data.OpsCommand-prev.OpsCommand) / delta

			data.NetInRate = float64(data.NetBytesIn-prev.NetBytesIn) / delta
			data.NetOutRate = float64(data.NetBytesOut-prev.NetBytesOut) / delta
		}
	}

	return data
}

func toInt64(v any) int64 {
	switch val := v.(type) {
	case int32:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	}
	return 0
}
