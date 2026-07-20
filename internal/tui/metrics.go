package tui

import (
	"context"
	"fmt"
	"sort"
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

type killCompletedMsg struct {
	OpID int64
	Err  error
}

type ActiveOpInfo struct {
	OpID      int64
	Namespace string
	Duration  string
	Op        string
}

type CollectionHotness struct {
	Namespace string
	TimeDelta float64
	Percent   float64
}

type metricsData struct {
	Time         time.Time
	MemVirtual   float64
	MemRes       float64
	ConnCurrent  int
	ConnAvail    int
	NetBytesIn   int64
	NetBytesOut  int64
	OpsInsert    int64
	OpsQuery     int64
	OpsUpdate    int64
	OpsDelete    int64
	OpsGetMore   int64
	OpsCommand   int64
	InsertRate   float64
	QueryRate    float64
	UpdateRate   float64
	DeleteRate   float64
	GetMoreRate  float64
	CommandRate  float64
	NetInRate    float64
	NetOutRate   float64
	ActiveOps    []ActiveOpInfo
	HottestColls []CollectionHotness
	topTimes     map[string]int64
}

type metricsModel struct {
	client        mongo.Client
	data          *metricsData
	err           error
	cursor        int
	confirmKill   bool
	killOpID      int64
	killNamespace string
	killOpType    string
}

func newMetricsModel(client mongo.Client) metricsModel {
	return metricsModel{client: client}
}

func (m metricsModel) initCmd() tea.Cmd {
	return pollMetricsCmd(m.client, nil, 0)
}

func (m metricsModel) Update(msg tea.Msg) (metricsModel, tea.Cmd) {
	if m.confirmKill {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				m.confirmKill = false
				opid := m.killOpID
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, err := m.client.RunAdminCommand(ctx, bson.D{
						{Key: "killOp", Value: 1},
						{Key: "op", Value: opid},
					})
					return killCompletedMsg{OpID: opid, Err: err}
				}
			case "n", "N", "esc":
				m.confirmKill = false
				return m, nil
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "m":
			return m, func() tea.Msg { return listBackMsg{} }
		case "j", "down":
			if m.data != nil && len(m.data.ActiveOps) > 0 {
				m.cursor++
				if m.cursor >= len(m.data.ActiveOps) {
					m.cursor = 0
				}
			}
		case "k", "up":
			if m.data != nil && len(m.data.ActiveOps) > 0 {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.data.ActiveOps) - 1
				}
			}
		case "d", "x":
			if m.data != nil && len(m.data.ActiveOps) > 0 && m.cursor >= 0 && m.cursor < len(m.data.ActiveOps) {
				op := m.data.ActiveOps[m.cursor]
				m.confirmKill = true
				m.killOpID = op.OpID
				m.killNamespace = op.Namespace
				m.killOpType = op.Op
			}
		}
	case killCompletedMsg:
		if msg.Err != nil {
			m.err = fmt.Errorf("error matando operación %d: %w", msg.OpID, msg.Err)
		}
		// Reset cursor and poll immediately
		m.cursor = 0
		return m, pollMetricsCmd(m.client, m.data, 0)

	case metricsPolledMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, pollMetricsCmd(m.client, m.data, 1*time.Second)
		}
		m.data = msg.Data
		m.err = nil

		if m.data != nil {
			if len(m.data.ActiveOps) == 0 {
				m.cursor = 0
			} else if m.cursor >= len(m.data.ActiveOps) {
				m.cursor = len(m.data.ActiveOps) - 1
			}
		}

		return m, pollMetricsCmd(m.client, m.data, 1*time.Second)
	}
	return m, nil
}

func (m metricsModel) View(width, height int) string {
	if m.confirmKill {
		var q strings.Builder
		q.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render("🚨 CONFIRMACIÓN: MATAR OPERACIÓN") + "\n\n")
		q.WriteString(fmt.Sprintf("¿Estás seguro de que deseas matar la operación %d?\n", m.killOpID))
		q.WriteString(fmt.Sprintf("Colección: %s\n", m.killNamespace))
		q.WriteString(fmt.Sprintf("Tipo:      %s\n\n", strings.ToUpper(m.killOpType)))
		q.WriteString("[y] Matar operación   [n/Esc] Cancelar")

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("9")).
			Padding(1, 2).
			Render(q.String())
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}

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

	// Layout columns
	leftWidth := (width - 10) / 2
	if leftWidth < 40 {
		leftWidth = 40
	}
	rightWidth := width - leftWidth - 8
	if rightWidth < 40 {
		rightWidth = 40
	}

	// LEFT COLUMN (Metrics rates, connections, network)
	var left strings.Builder
	left.WriteString(lipgloss.NewStyle().Bold(true).Render("MEMORIA") + "\n")
	memMax := m.data.MemVirtual
	if memMax < 16.0 {
		memMax = 16.0
	}
	memBar := renderProgressBar(m.data.MemRes, memMax, 18, "2", "8")
	left.WriteString(fmt.Sprintf("  Virtual:   %.2f GB\n", m.data.MemVirtual))
	left.WriteString(fmt.Sprintf("  Residente: %.2f GB\n", m.data.MemRes))
	left.WriteString(fmt.Sprintf("  Uso:       %s\n\n", memBar))

	left.WriteString(lipgloss.NewStyle().Bold(true).Render("CONEXIONES") + "\n")
	connMax := float64(m.data.ConnCurrent + m.data.ConnAvail)
	connBar := renderProgressBar(float64(m.data.ConnCurrent), connMax, 18, "6", "8")
	left.WriteString(fmt.Sprintf("  Activas:     %d\n", m.data.ConnCurrent))
	left.WriteString(fmt.Sprintf("  Disponibles: %d\n", m.data.ConnAvail))
	left.WriteString(fmt.Sprintf("  Uso:         %s\n\n", connBar))

	left.WriteString(lipgloss.NewStyle().Bold(true).Render("OPERACIONES POR SEGUNDO") + "\n")
	left.WriteString(fmt.Sprintf("  Insertar:    %6.1f ops/s  |  Consulta:  %6.1f ops/s\n", m.data.InsertRate, m.data.QueryRate))
	left.WriteString(fmt.Sprintf("  Actualizar:  %6.1f ops/s  |  Borrar:    %6.1f ops/s\n", m.data.UpdateRate, m.data.DeleteRate))
	left.WriteString(fmt.Sprintf("  Comando:     %6.1f ops/s  |  GetMore:   %6.1f ops/s\n\n", m.data.CommandRate, m.data.GetMoreRate))

	left.WriteString(lipgloss.NewStyle().Bold(true).Render("RENDIMIENTO DE RED") + "\n")
	left.WriteString(fmt.Sprintf("  Entrada:     %s\n", formatBytesRate(m.data.NetInRate)))
	left.WriteString(fmt.Sprintf("  Salida:      %s\n", formatBytesRate(m.data.NetOutRate)))

	// RIGHT COLUMN (Hottest Collections, Slowest Ops)
	var right strings.Builder
	right.WriteString(lipgloss.NewStyle().Bold(true).Render("COLECCIONES MÁS ACTIVAS (Hottest)") + "\n")
	if len(m.data.HottestColls) == 0 {
		right.WriteString("  (sin actividad de lectura/escritura en colecciones)\n\n")
	} else {
		for _, hc := range m.data.HottestColls {
			ns := hc.Namespace
			if len(ns) > rightWidth-12 {
				ns = ns[:rightWidth-15] + "..."
			}
			right.WriteString(fmt.Sprintf("  %-*s %5.1f%%\n", rightWidth-12, ns, hc.Percent))
		}
		right.WriteString("\n")
	}

	right.WriteString(lipgloss.NewStyle().Bold(true).Render("OPERACIONES ACTIVAS (currentOp)") + "\n")
	if len(m.data.ActiveOps) == 0 {
		right.WriteString("  (ninguna operación activa en ejecución)\n")
	} else {
		right.WriteString(fmt.Sprintf("    %-8s %-*s %-10s\n", "OPID", rightWidth-24, "COLECCIÓN", "DURACIÓN"))

		// Sliding window for operations list
		startIdx := 0
		if m.cursor >= 4 {
			startIdx = m.cursor - 3
		}
		endIdx := startIdx + 4
		if endIdx > len(m.data.ActiveOps) {
			endIdx = len(m.data.ActiveOps)
			startIdx = endIdx - 4
			if startIdx < 0 {
				startIdx = 0
			}
		}

		for idx := startIdx; idx < endIdx; idx++ {
			op := m.data.ActiveOps[idx]
			marker := " "
			if idx == m.cursor {
				marker = lipgloss.NewStyle().Foreground(focusedBorderColor).Render(">")
			}
			ns := op.Namespace
			if len(ns) > rightWidth-24 {
				ns = ns[:rightWidth-27] + "..."
			}
			right.WriteString(fmt.Sprintf("  %s %-8d %-*s %-10s\n", marker, op.OpID, rightWidth-24, ns, op.Duration))
		}
		if len(m.data.ActiveOps) > 4 {
			right.WriteString(fmt.Sprintf("  (mostrando %d de %d operaciones activas)\n", endIdx-startIdx, len(m.data.ActiveOps)))
		}
	}

	// Composite view
	leftCol := lipgloss.NewStyle().Width(leftWidth).Render(left.String())
	rightCol := lipgloss.NewStyle().Width(rightWidth).Render(right.String())
	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)

	var b strings.Builder
	// Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(focusedBorderColor)
	if m.err != nil {
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
		b.WriteString(headerStyle.Render("=== MONITOREO DE RENDIMIENTO DE MONGODB ===") + "  " + warnStyle.Render("[⚠️ ERROR: Reintentando...]") + "\n\n")
	} else {
		b.WriteString(headerStyle.Render("=== MONITOREO DE RENDIMIENTO DE MONGODB ===") + "\n\n")
	}

	b.WriteString(mainRow + "\n\n")
	b.WriteString(helpHintStyle.Render("[j/k/↑/↓] Seleccionar OP  [d/x] Matar OP  [Esc/q] Volver a lazymongo"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusedBorderColor).
		Padding(1, 2).
		Width(width - 4).
		Height(height - 2).
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

		// Query top command for hottest collections
		var hottestColls []CollectionHotness
		currentTimes := make(map[string]int64)
		topRaw, topErr := client.RunAdminCommand(ctx, bson.D{{Key: "top", Value: 1}})
		if topErr == nil {
			if totalsVal, ok := toMap(topRaw["totals"]); ok {
				for ns, nsVal := range totalsVal {
					if nsMap, ok := toMap(nsVal); ok {
						if totalMap, ok := toMap(nsMap["total"]); ok {
							if tVal, ok := totalMap["time"]; ok {
								currentTimes[ns] = toInt64(tVal)
							}
						}
					}
				}

				var deltas []CollectionHotness
				var sumDeltas float64
				if prev != nil && prev.topTimes != nil {
					for ns, currTime := range currentTimes {
						prevTime := prev.topTimes[ns]
						delta := currTime - prevTime
						if delta < 0 {
							delta = 0
						}
						if delta > 0 {
							deltas = append(deltas, CollectionHotness{
								Namespace: ns,
								TimeDelta: float64(delta),
							})
							sumDeltas += float64(delta)
						}
					}
				}

				if sumDeltas > 0 {
					for i := range deltas {
						deltas[i].Percent = (deltas[i].TimeDelta / sumDeltas) * 100.0
					}
					sort.Slice(deltas, func(i, j int) bool {
						return deltas[i].Percent > deltas[j].Percent
					})
					hottestColls = deltas
				} else {
					for ns, totalTime := range currentTimes {
						if totalTime > 0 {
							deltas = append(deltas, CollectionHotness{
								Namespace: ns,
								TimeDelta: float64(totalTime),
							})
						}
					}
					sort.Slice(deltas, func(i, j int) bool {
						return deltas[i].TimeDelta > deltas[j].TimeDelta
					})
					hottestColls = deltas
				}
				if len(hottestColls) > 5 {
					hottestColls = hottestColls[:5]
				}
			}
		}

		// Query currentOp for active slow operations
		var activeOps []ActiveOpInfo
		opRaw, err := client.RunAdminCommand(ctx, bson.D{
			{Key: "currentOp", Value: 1},
			{Key: "active", Value: true},
		})
		if err == nil {
			if inprog, ok := toArray(opRaw["inprog"]); ok {
				for _, opVal := range inprog {
					if opMap, ok := toMap(opVal); ok {
						opid := int64(0)
						if idVal, ok := opMap["opid"]; ok {
							opid = toInt64(idVal)
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
							durationUs = toInt64(durVal)
						}
						durationStr := fmt.Sprintf("%.2f ms", float64(durationUs)/1000.0)
						if durationUs > 1000000 {
							durationStr = fmt.Sprintf("%.2f s", float64(durationUs)/1000000.0)
						}
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
		data.HottestColls = hottestColls
		data.topTimes = currentTimes
		return metricsPolledMsg{Data: data}
	}
}

func parseServerStatus(status bson.M, prev *metricsData) *metricsData {
	data := &metricsData{
		Time: time.Now(),
	}

	if mem, ok := toMap(status["mem"]); ok {
		if v, ok := mem["virtual"]; ok {
			data.MemVirtual = toFloat64(v) / 1024.0
		}
		if r, ok := mem["resident"]; ok {
			data.MemRes = toFloat64(r) / 1024.0
		}
	}

	if conns, ok := toMap(status["connections"]); ok {
		if c, ok := conns["current"]; ok {
			data.ConnCurrent = int(toInt64(c))
		}
		if a, ok := conns["available"]; ok {
			data.ConnAvail = int(toInt64(a))
		}
	}

	if net, ok := toMap(status["network"]); ok {
		if bi, ok := net["bytesIn"]; ok {
			data.NetBytesIn = toInt64(bi)
		}
		if bo, ok := net["bytesOut"]; ok {
			data.NetBytesOut = toInt64(bo)
		}
	}

	if ops, ok := toMap(status["opcounters"]); ok {
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
	case int:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case float32:
		return int64(val)
	case uint32:
		return int64(val)
	case uint64:
		return int64(val)
	}
	return 0
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case int:
		return float64(val)
	}
	return 0
}
