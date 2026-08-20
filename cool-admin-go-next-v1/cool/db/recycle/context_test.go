package recycle

import (
	"context"
	"testing"
)

func TestBypassAndRequestMetadataUseTypedContextKeys(t *testing.T) {
	tenantID := int64(7)
	ctx := WithRequestMetadata(context.Background(), RequestMetadata{
		UserID: 9, TenantID: &tenantID, URL: "/admin/demo/delete", Method: "POST", Params: []byte(`{"ids":[1]}`),
	})
	if IsBypass(ctx) {
		t.Fatal("request metadata must not enable recycle bypass")
	}
	metadata, ok := RequestMetadataFromContext(ctx)
	if !ok || metadata.UserID != 9 || metadata.TenantID == nil || *metadata.TenantID != 7 {
		t.Fatalf("unexpected request metadata: %#v", metadata)
	}
	bypassCtx := WithBypass(ctx)
	if !IsBypass(bypassCtx) || IsBypass(context.Background()) {
		t.Fatal("typed recycle bypass state was not isolated")
	}
}
