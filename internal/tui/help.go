package tui

const helpText = `lazymongo — atajos

j/k, flechas   moverse
Enter          entrar / seleccionar
Esc, h         volver un nivel
/              filtrar (en Documentos)
Tab            alternar Documentos/Índices
n/p            página siguiente/anterior
i, a           insertar documento / crear conexión o índice
e              editar campo (en detalle de documento)
E              editar documento completo en $EDITOR
d, x           borrar (siempre pide confirmación)
?              esta ayuda
Ctrl+c         salir

Presiona cualquier tecla para cerrar esta ayuda.`

type helpModel struct{}

func (m helpModel) View() string { return helpText }
