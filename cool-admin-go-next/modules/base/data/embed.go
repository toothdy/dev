// Package data 提供 Base 模块内置的初始化数据。
package data

import _ "embed"

var (
	//go:embed db.json
	dbSeed []byte
	//go:embed menu.json
	menuSeed []byte
)

// DBSeed 返回数据库初始化数据副本。
func DBSeed() []byte {
	return append([]byte(nil), dbSeed...)
}

// MenuSeed 返回菜单初始化数据副本。
func MenuSeed() []byte {
	return append([]byte(nil), menuSeed...)
}
