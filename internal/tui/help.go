package tui

import (
	"strings"
)

type helpModel struct {
	focus panelID
}

func (m helpModel) View() string {
	var b strings.Builder

	var panelName string
	var specificHelp string

	switch m.focus {
	case panelStatus:
		panelName = "Status / Estado (Panel 1)"
		specificHelp = `  j/k, flechas   navegar por el historial de comandos/logs
  (Este panel muestra el estado actual de la conexión y las bases de datos)`

	case panelDatabases:
		panelName = "Bases de Datos (Panel 2)"
		specificHelp = `  j/k, flechas   navegar por las bases de datos (carga colecciones automáticamente)
  /              buscar base de datos (filtro fuzzy interactivo)
  a, i           crear una nueva base de datos (solicita nombre y primera colección)
  d, x           borrar la base de datos seleccionada (requiere confirmación)`

	case panelCollections:
		panelName = "Colecciones (Panel 3)"
		specificHelp = `  j/k, flechas   navegar por las colecciones (carga documentos e índices automáticamente)
  /              buscar colección (filtro fuzzy interactivo)
  a, i           crear una nueva colección en la base de datos activa
  e              renombrar la colección seleccionada (edición con cursor de flechas)
  d, x           borrar la colección seleccionada (requiere confirmación)`

	case panelIndexes:
		panelName = "Índices (Panel 4)"
		specificHelp = `  j/k, flechas   navegar por los índices de la colección activa
  /              buscar índice (filtro fuzzy interactivo)
  a, i           crear un nuevo índice (solicita llaves JSON y si es único)
  d, x           borrar el índice seleccionado (requiere confirmación)`

	case panelConnections:
		panelName = "Conexiones (Panel 5)"
		specificHelp = `  j/k, flechas   navegar por las conexiones de tu configuración
  /              buscar conexión (filtro fuzzy interactivo)
  Enter          conectar al servidor de base de datos seleccionado
  a, i           crear una nueva conexión (nombre, URI y color del borde)
  e              editar la conexión seleccionada (edición con cursor de flechas)
  d, x           borrar la conexión de tu configuración (requiere confirmación)`

	case panelDocuments:
		panelName = "Documentos (Panel Principal)"
		specificHelp = `  j/k, flechas   navegar por la lista de documentos (expande/colapsa automáticamente el seleccionado)
  Enter          abrir el visor interactivo de detalle del documento:
                 - j/k, flechas : moverse entre los campos del documento
                 - e            : editar el valor del campo seleccionado
                 - y, c         : copiar el valor seleccionado al portapapeles (hex para ObjectIDs)
                 - E            : editar el documento completo como JSON en tu $EDITOR
                 - d, x         : borrar campo del documento (requiere confirmación)
                 - Esc          : volver a la lista de documentos
  /              escribir filtro BSON/JSON de consulta (ej. {"customer_id": 11876068})
                 - Tab : autocompletar nombres de campos según tus documentos
                 - Esc : limpiar filtro activo
  s              escribir ordenamiento BSON/JSON (ej. {"createdAt": -1})
                 - Tab : autocompletar nombres de campos según tus documentos
                 - Esc : limpiar orden activo
  Ctrl+f         iniciar búsqueda local rápida (fuzzy find) sobre los documentos cargados
  n/p            página siguiente / anterior
  i, a           insertar un nuevo documento desde tu $EDITOR
  d, x           borrar el documento seleccionado (requiere confirmación)`

	default:
		panelName = "General"
		specificHelp = `  j/k, flechas   moverse dentro del panel activo
  Enter          confirmar / ver detalle / entrar`
	}

	b.WriteString(titleStyle.Render("lazymongo — Ayuda: "+panelName) + "\n\n")
	b.WriteString("Atajos Globales:\n")
	b.WriteString("  0-5            saltar a un panel (0: Documentos, 1: Status, 2: DBs, 3: Colls, 4: Índices, 5: Conexiones)\n")
	b.WriteString("  Tab            ir al panel de Índices (solo si estás en Documentos)\n")
	b.WriteString("  Esc            cerrar popups activos / cancelar modo de búsqueda o entrada de texto\n")
	b.WriteString("  r              refrescar/recargar datos del panel activo\n")
	b.WriteString("  m              abrir el monitor de métricas de rendimiento en tiempo real (Compass)\n")
	b.WriteString("  ?              abrir/cerrar esta ayuda\n")
	b.WriteString("  Ctrl+c         salir de la aplicación\n\n")

	b.WriteString("Atajos Específicos para este Panel:\n")
	b.WriteString(specificHelp + "\n\n")

	b.WriteString(helpHintStyle.Render("Presiona cualquier tecla para cerrar esta ayuda."))

	return b.String()
}
