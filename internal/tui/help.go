package tui

const helpText = `lazymongo — atajos

1-5            saltar a un panel (Status/Databases/Collections/Indexes/Conexiones)
Tab            saltar a Documentos (o a Indexes si ya estás en Documentos)
j/k, flechas   moverse dentro del panel enfocado
Enter          ver documento / conectar (en Conexiones) / entrar
Esc            cerrar el popup activo / salir de un buscador activo
/              buscar (fuzzy en Databases/Collections/Indexes/Conexiones; filtro de query Mongo en Documentos)
Ctrl+f         buscar (fuzzy) entre los documentos ya cargados en pantalla
n/p            página siguiente/anterior (en Documentos)
i, a           insertar documento / crear conexión, índice, database o collection
e              editar campo (en detalle de documento) / editar conexión (nombre, URI y color) / renombrar collection
E              editar documento completo en $EDITOR
d, x           borrar (siempre pide confirmación)
?              esta ayuda
Ctrl+c         salir

Presiona cualquier tecla para cerrar esta ayuda.`

type helpModel struct{}

func (m helpModel) View() string { return helpText }
