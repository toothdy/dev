package database

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/driver"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/schema"
	baseentity "github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

func TestUserTransientSchemaMatrix(t *testing.T) {
	config, err := loadConfig(os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !config.enabled {
		t.Skip("未启用数据库集成测试")
	}
	descriptor, err := coreentity.Compile[baseentity.User, uint64](baseentity.UserSchema())
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range smokeCases(config) {
		t.Run(string(testCase.kind), func(t *testing.T) {
			database, databaseErr := gdb.New(testCase.node)
			if databaseErr != nil {
				t.Fatal(databaseErr)
			}
			report, databaseErr := driver.Probe(t.Context(), database)
			if databaseErr != nil {
				t.Fatal(databaseErr)
			}
			quotedTable, databaseErr := report.Dialect.Quote(descriptor.Table())
			if databaseErr != nil {
				t.Fatal(databaseErr)
			}
			_, _ = database.Exec(t.Context(), "DROP TABLE IF EXISTS "+quotedTable)
			t.Cleanup(func() {
				_, _ = database.Exec(context.Background(), "DROP TABLE IF EXISTS "+quotedTable)
			})
			manager, databaseErr := schema.New(database, report.Dialect)
			if databaseErr != nil {
				t.Fatal(databaseErr)
			}
			if _, databaseErr = manager.Apply(t.Context(), schema.Sync, descriptor); databaseErr != nil {
				t.Fatal(databaseErr)
			}
			if _, databaseErr = manager.Apply(t.Context(), schema.Validate, descriptor); databaseErr != nil {
				t.Fatal(databaseErr)
			}
			columns, databaseErr := userTableColumns(t.Context(), database, testCase.kind, descriptor.Table())
			if databaseErr != nil {
				t.Fatal(databaseErr)
			}
			if columns["roleIdList"] {
				t.Fatal("transient roleIdList 被写入用户表")
			}
			if !columns["password"] {
				t.Fatal("用户表缺少 password 持久化列")
			}
		})
	}
}

func userTableColumns(ctx context.Context, database gdb.DB, kind databaseKind, table string) (map[string]bool, error) {
	var (
		rows []struct {
			Name string `orm:"name"`
		}
		err error
	)
	switch kind {
	case databaseMySQL:
		err = database.Model("information_schema.columns").Ctx(ctx).
			Fields("column_name AS name").Where("table_schema = DATABASE()").Where("table_name", table).Scan(&rows)
	case databasePostgreSQL:
		err = database.Model("information_schema.columns").Ctx(ctx).
			Fields("column_name AS name").Where("table_schema = CURRENT_SCHEMA()").Where("table_name", table).Scan(&rows)
	case databaseSQLite:
		err = database.Raw("SELECT name FROM pragma_table_info(?)", table).Scan(&rows)
	}
	result := make(map[string]bool, len(rows))
	for _, row := range rows {
		result[row.Name] = true
	}

	return result, err
}
