declare -A MONGO_CONNECTIONS=(
  [qa]="mongodb://localhost:27017/test"
  [prod]="mongodb://localhost:27017/prod"
)

declare -A MONGO_CONNECTION_COLORS=(
  [qa]="verde"
  [prod]="rojo"
)
