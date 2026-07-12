package tui

const helpText = `lazymongo — atajos

1-5            saltar a un panel (Status/Databases/Collections/Indexes/Conexiones)
Tab            saltar al panel de Documentos
j/k, flechas   moverse dentro del panel enfocado
Enter          ver documento / conectar (en Conexiones) / entrar
Esc            cerrar el popup activo
/              filtrar (en Documentos)
n/p            página siguiente/anterior (en Documentos)
i, a           insertar documento / crear conexión o índice
e              editar campo (en detalle de documento)
E              editar documento completo en $EDITOR
d, x           borrar (siempre pide confirmación)
?              esta ayuda
Ctrl+c         salir

Presiona cualquier tecla para cerrar esta ayuda.`

type helpModel struct{}

func (m helpModel) View() string { return helpText }
