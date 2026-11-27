package migrations

import (
	"embed"

	"plenka/kit/pgkit"
)

//go:embed *.sql
var FS embed.FS

// Deny — чего роль данных не получает: в outbox сервис только добавляет
// события, а читает и отмечает их ретранслятор под ролью владельца схемы.
// Так SQL-инъекция под ролью данных не прочитает и не сотрёт события.
var Deny = []pgkit.Deny{{Table: "outbox", Privs: "SELECT, UPDATE, DELETE"}}
