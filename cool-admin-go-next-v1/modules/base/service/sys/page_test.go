package sys

import (
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

func TestPageLimitsMatchNodeEntityAndSQLExportBehavior(t *testing.T) {
	request := crud.QueryRequest{Page: 2, Size: 15, IsExport: true}
	entitySQL, entityArgs := pageLimit(request)
	if entitySQL != " LIMIT ?" || !reflect.DeepEqual(entityArgs, []interface{}{crud.MaxExportSize}) {
		t.Fatalf("unexpected entity export pagination: %q %#v", entitySQL, entityArgs)
	}
	sql, args := sqlPageLimit(request)
	if sql != " LIMIT ?" || !reflect.DeepEqual(args, []interface{}{crud.MaxExportSize}) {
		t.Fatalf("unexpected SQL export pagination: %q %#v", sql, args)
	}
}
