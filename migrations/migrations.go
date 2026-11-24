// Package migrations содержит SQL-файлы миграций и встраивает их в бинарник
// (go:embed), чтобы миграция выполнялась при старте приложения без внешних
// инструментов.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var sqlFS embed.FS

// FS — файловая система с SQL-миграциями.
var FS fs.FS = sqlFS
