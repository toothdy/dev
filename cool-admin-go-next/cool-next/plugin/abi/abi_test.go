package abi

import (
	"reflect"
	"testing"
)

func TestGuestExports(t *testing.T) {
	want := []FunctionSignature{
		{Name: ExportABIVersion, Results: []ValueType{ValueTypeI32}},
		{Name: ExportAlloc, Parameters: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI32}},
		{Name: ExportFree, Parameters: []ValueType{ValueTypeI32, ValueTypeI32}},
		{Name: ExportInit, Parameters: []ValueType{ValueTypeI64, ValueTypeI32, ValueTypeI32}, Results: []ValueType{ValueTypeI64}},
		{Name: ExportInvoke, Parameters: []ValueType{ValueTypeI64, ValueTypeI32, ValueTypeI32, ValueTypeI32, ValueTypeI32}, Results: []ValueType{ValueTypeI64}},
		{Name: ExportShutdown, Parameters: []ValueType{ValueTypeI64}, Results: []ValueType{ValueTypeI64}},
		{Name: ExportResponsePointer, Parameters: []ValueType{ValueTypeI64}, Results: []ValueType{ValueTypeI32}},
		{Name: ExportResponseLength, Parameters: []ValueType{ValueTypeI64}, Results: []ValueType{ValueTypeI32}},
		{Name: ExportResponseDrop, Parameters: []ValueType{ValueTypeI64}},
	}
	actual := GuestExports()
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("GuestExports() = %#v, want %#v", actual, want)
	}

	actual[0].Name = "changed"
	actual[1].Parameters[0] = ValueTypeI64
	if !reflect.DeepEqual(GuestExports(), want) {
		t.Fatal("GuestExports() 未返回防御性副本")
	}
}

func TestHostImports(t *testing.T) {
	want := []FunctionSignature{
		{Name: ImportCall, Parameters: []ValueType{ValueTypeI64, ValueTypeI32, ValueTypeI32, ValueTypeI32, ValueTypeI32}, Results: []ValueType{ValueTypeI64}},
		{Name: ImportResponseLength, Parameters: []ValueType{ValueTypeI64}, Results: []ValueType{ValueTypeI32}},
		{Name: ImportResponseRead, Parameters: []ValueType{ValueTypeI64, ValueTypeI32, ValueTypeI32}, Results: []ValueType{ValueTypeI32}},
		{Name: ImportResponseDrop, Parameters: []ValueType{ValueTypeI64}},
	}
	actual := HostImports()
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("HostImports() = %#v, want %#v", actual, want)
	}

	actual[0].Results[0] = ValueTypeI32
	if !reflect.DeepEqual(HostImports(), want) {
		t.Fatal("HostImports() 未返回防御性副本")
	}
}
