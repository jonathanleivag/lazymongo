# Conexiones de MongoDB nombradas, usadas por la función `mgo` de .zshrc.
# Este archivo NO se sube a ningún repo — solo vive en esta Mac.
# Formato: [nombre]="uri completa de mongodb"

declare -A MONGO_CONNECTIONS=(
  [ejemplo-local]="mongodb://localhost:27017"
)
