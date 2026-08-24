package crud

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/frame/g"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

type projectionRoot struct {
	g.Meta `orm:"table:projection_root" description:"投影根实体"`
	coreentity.Base
	OwnerID uint64 `json:"ownerId" orm:"ownerId" description:"所有者"`
	Name    string `json:"name" orm:"displayName" description:"名称"`
	Status  int    `json:"status" orm:"status" description:"状态"`
}

type projectionOwner struct {
	g.Meta `orm:"table:projection_owner" description:"投影所有者实体"`
	coreentity.Base
	Name string `json:"name" orm:"ownerName" description:"名称"`
}

type projectionResolver struct {
	root  coreentity.Metadata
	owner coreentity.Metadata
}

func (resolver projectionResolver) Resolve(value any) (coreentity.Metadata, bool) {
	switch value.(type) {
	case projectionRoot:
		return resolver.root, true
	case projectionOwner:
		return resolver.owner, true
	default:
		return nil, false
	}
}

func TestProjectQueryResolvesStaticFields(t *testing.T) {
	root, owner, resolver := compileProjectionFixtures(t)
	join := LeftJoin(projectionOwner{}, "owner", On(
		NewColumnRef("ownerId"),
		NewColumnRefOf[projectionOwner]("id").Of("owner"),
	))
	op := QueryOp{
		Join: []JoinOp{join},
		KeyWordLikeFields: []ColumnRef{
			NewColumnRef("name"),
			NewColumnRefOf[projectionOwner]("name").Of("owner"),
		},
		FieldEq: []FieldEq{
			EqFrom(NewColumnRef("status"), "state"),
		},
		FieldLike: []FieldLike{
			LikeFrom(NewColumnRefOf[projectionOwner]("name").Of("owner"), "ownerKeyword"),
		},
		Select: []SelectField{
			All("a"),
			As(NewColumnRefOf[projectionOwner]("name").Of("owner"), "ownerName"),
			As(NewColumnRef("name"), "rootNameAgain"),
		},
	}

	projection, err := ProjectQuery(resolver, projectionRoot{}, op)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.KeyWordLikeFields) != 2 || projection.KeyWordLikeFields[1].Source != "owner.name" {
		t.Fatalf("keyword fields = %#v", projection.KeyWordLikeFields)
	}
	if len(projection.FieldEq) != 1 || projection.FieldEq[0].Column.Source != "a.status" || projection.FieldEq[0].RequestParam != "state" {
		t.Fatalf("field eq = %#v", projection.FieldEq)
	}
	if len(projection.FieldLike) != 1 || projection.FieldLike[0].Column.Descriptor != owner || projection.FieldLike[0].RequestParam != "ownerKeyword" {
		t.Fatalf("field like = %#v", projection.FieldLike)
	}
	if len(projection.Select) != len(root.Fields())+2 {
		t.Fatalf("select count = %d", len(projection.Select))
	}
	ownerSelect := projection.Select[len(root.Fields())]
	rootAliasSelect := projection.Select[len(root.Fields())+1]
	if ownerSelect.Name != "ownerName" || ownerSelect.Column.Descriptor != owner || ownerSelect.Column.Source != "owner.name" {
		t.Fatalf("owner select = %#v", ownerSelect)
	}
	if rootAliasSelect.Name != "rootNameAgain" || rootAliasSelect.Column.Source != "a.name" {
		t.Fatalf("root alias select = %#v", rootAliasSelect)
	}
	if projection.Select[0].Column.Field == nil || projection.Select[0].Column.Descriptor != root {
		t.Fatalf("root select = %#v", projection.Select[0])
	}
}

func TestProjectQueryDoesNotExecuteExtendAndReturnsIndependentSlices(t *testing.T) {
	_, _, resolver := compileProjectionFixtures(t)
	isCalled := false
	op := QueryOp{
		KeyWordLikeFields: []ColumnRef{NewColumnRef("name")},
		Extend: func(_ context.Context, _ *QueryBuilder, _ *QueryRequest) error {
			isCalled = true
			return nil
		},
	}

	first, err := ProjectQuery(resolver, projectionRoot{}, op)
	if err != nil {
		t.Fatal(err)
	}
	if isCalled {
		t.Fatal("ProjectQuery() 执行了 Extend")
	}
	first.KeyWordLikeFields[0].Source = "changed"
	second, err := ProjectQuery(resolver, projectionRoot{}, op)
	if err != nil {
		t.Fatal(err)
	}
	if second.KeyWordLikeFields[0].Source != "a.name" || len(second.Select) != 0 {
		t.Fatalf("second projection = %#v", second)
	}
}

func TestProjectQueryRejectsInvalidProjectedFields(t *testing.T) {
	_, _, resolver := compileProjectionFixtures(t)
	tests := []QueryOp{
		{KeyWordLikeFields: []ColumnRef{NewColumnRef("status")}},
		{FieldLike: []FieldLike{Like(NewColumnRef("status"))}},
		{FieldEq: []FieldEq{{Column: NewColumnRef("missing"), RequestParam: "missing"}}},
		{Select: []SelectField{All("missing")}},
	}
	for _, op := range tests {
		if _, err := ProjectQuery(resolver, projectionRoot{}, op); err == nil {
			t.Fatalf("ProjectQuery(%#v) error = nil", op)
		}
	}
}

func TestProjectColumnsResolvesOnlyRootEntity(t *testing.T) {
	_, _, resolver := compileProjectionFixtures(t)
	columns, err := ProjectColumns(resolver, projectionRoot{}, []ColumnRef{
		NewColumnRef("name"),
		NewColumnRefOf[projectionRoot]("status").Of("a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 2 || columns[0].Source != "a.name" || columns[1].Field.Name() != "status" {
		t.Fatalf("columns = %#v", columns)
	}
	columns[0].Source = "changed"
	second, err := ProjectColumns(resolver, projectionRoot{}, []ColumnRef{NewColumnRef("name")})
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Source != "a.name" {
		t.Fatalf("second columns = %#v", second)
	}

	invalid := []ColumnRef{
		NewColumnRef("missing"),
		NewColumnRefOf[projectionOwner]("name"),
		NewColumnRef("name").Of("owner"),
	}
	for _, column := range invalid {
		if _, err = ProjectColumns(resolver, projectionRoot{}, []ColumnRef{column}); err == nil {
			t.Fatalf("ProjectColumns(%#v) error = nil", column)
		}
	}
}

func compileProjectionFixtures(t *testing.T) (coreentity.Metadata, coreentity.Metadata, DescriptorResolver) {
	t.Helper()
	root, err := coreentity.Compile[projectionRoot, uint64](coreentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := coreentity.Compile[projectionOwner, uint64](coreentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}

	return root, owner, projectionResolver{root: root, owner: owner}
}
