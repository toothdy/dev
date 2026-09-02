package gnctrl

import (
	"context"
	"net/http"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
)

type projectionEntity struct {
	g.Meta `orm:"table:projection_entity" description:"投影实体"`
	gnentity.Base
	Name string `json:"name" orm:"name" description:"名称"`
}

type projectionService struct{}

func TestSnapshotReturnsIndependentDefinitionCopy(t *testing.T) {
	definition := Admin("users").
		Options(RouterOptions{
			Alias:      []string{"user"},
			Middleware: []MiddlewareRef{"auth.guard"},
		}).
		Curd(CurdOption{
			Prefix:       "archive/users",
			API:          API(Page),
			Entity:       projectionEntity{},
			Service:      &projectionService{},
			HiddenFields: []ColumnRef{Field("name")},
			PageQueryOp: StaticQuery(QueryOp{
				FieldLike: []FieldLike{Like(Field("name"))},
			}),
		}).
		Route(Route{
			Method:  http.MethodGet,
			Path:    "/export",
			Handler: Handle(func() {}),
			Tags:    []URLTag{{Name: TagIgnoreToken}},
		}).
		Build()

	first, err := Snapshot(definition)
	if err != nil {
		t.Fatal(err)
	}
	if first.Area != AreaAdmin || first.Path != "users" || first.Curd == nil || len(first.Routes) != 1 {
		t.Fatalf("snapshot = %#v", first)
	}
	first.Options.Alias[0] = "changed"
	first.Options.Middleware[0] = "changed.guard"
	first.Curd.API[0] = Add
	first.Curd.HiddenFields[0] = Field("id")
	first.Routes[0].Tags[0].Name = "changed"

	second, err := Snapshot(definition)
	if err != nil {
		t.Fatal(err)
	}
	if second.Options.Alias[0] != "user" || second.Options.Middleware[0] != "auth.guard" {
		t.Fatalf("options = %#v", second.Options)
	}
	if second.Curd.API[0] != Page || second.Curd.HiddenFields[0] != Field("name") {
		t.Fatalf("curd = %#v", second.Curd)
	}
	if second.Routes[0].Tags[0].Name != TagIgnoreToken {
		t.Fatalf("routes = %#v", second.Routes)
	}
}

func TestProjectQueryDistinguishesStaticAndDynamic(t *testing.T) {
	descriptor, err := gnentity.Compile[projectionEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	resolver := DescriptorResolverFunc(func(value any) (gnentity.Metadata, bool) {
		_, matches := value.(projectionEntity)
		return descriptor, matches
	})

	static, isStatic, err := ProjectQuery(StaticQuery(QueryOp{
		FieldLike: []FieldLike{LikeFrom(Field("name"), "keyword")},
	}), resolver, projectionEntity{})
	if err != nil {
		t.Fatal(err)
	}
	if !isStatic || len(static.FieldLike) != 1 || static.FieldLike[0].RequestParam != "keyword" {
		t.Fatalf("static projection = %#v, %t", static, isStatic)
	}

	isCalled := false
	dynamic, isStatic, err := ProjectQuery(DynamicQuery(func(context.Context) (QueryOp, error) {
		isCalled = true
		return QueryOp{}, nil
	}), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if isCalled || isStatic || len(dynamic.Select) != 0 {
		t.Fatalf("dynamic projection = %#v, static = %t, called = %t", dynamic, isStatic, isCalled)
	}
}

func TestSnapshotRejectsInvalidDefinition(t *testing.T) {
	if _, err := Snapshot(nil); err == nil {
		t.Fatal("Snapshot(nil) error = nil")
	}
}
