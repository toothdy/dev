package base_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	eps "github.com/toothdy/cool-admin-go-next/cool/util/eps"
)

func TestEPSIntegrationAnonymousBootstrap(t *testing.T) {
	if os.Getenv("COOL_EPS_INTEGRATION") != "1" {
		t.Skip("set COOL_EPS_INTEGRATION=1 to run EPS HTTP integration test")
	}

	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		Specs:        applicationSpecs(),
		AuthManagerFactory: baseTestAuthManagerFactory,
		SessionStore:       security.NewMemorySessionStore(),
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start EPS test server failed: %v", err)
	}
	defer server.Shutdown()

	response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort()) + "/admin/base/open/eps")
	if err != nil {
		t.Fatalf("request EPS failed: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read EPS response failed: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.StatusCode, body)
	}
	decoded := struct {
		Code int                         `json:"code"`
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err = json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode EPS response failed: %v", err)
	}
	if decoded.Code != exception.CodeSuccess || len(decoded.Data["base"]) != 8 {
		t.Fatalf("unexpected EPS response: %#v", decoded)
	}
}
