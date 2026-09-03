package gnhttp

import (
	"context"
	"errors"
	"io"
	"net"
	stdhttp "net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/app"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/config"
)

type readyProbe struct {
	ready atomic.Bool
}

func (probe *readyProbe) Ready() bool { return probe.ready.Load() }

type runtimeProbe struct {
	listener      net.Listener
	listenerSet   atomic.Bool
	shutdown      <-chan struct{}
	shutdownCalls atomic.Int32
	shutdownErr   error
	startErr      error
	status        int
}

func (*runtimeProbe) Configure(app.ReadyState, Installer) error { return nil }

func (probe *runtimeProbe) SetListener(listener net.Listener) error {
	probe.listener = listener
	probe.listenerSet.Store(true)
	return nil
}

func (probe *runtimeProbe) Start() error {
	if !probe.listenerSet.Load() {
		return errors.New("start called before listener was set")
	}
	return probe.startErr
}

func (probe *runtimeProbe) Status() int { return probe.status }

func (probe *runtimeProbe) Shutdown() error {
	probe.shutdownCalls.Add(1)
	if probe.shutdown != nil {
		<-probe.shutdown
	}
	return probe.shutdownErr
}

func TestLoadConfigMergesDefaultsAndValidates(t *testing.T) {
	config, err := LoadConfig(t.Context(), config.Source{Main: []byte(`
enabled: false
address: 127.0.0.1
port: 9000
startTimeout: 250ms
`)})
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.Address != "127.0.0.1" || config.Port != 9000 ||
		config.StartTimeout != 250*time.Millisecond || config.ClientMaxBodySize != DefaultClientMaxBodySize {
		t.Fatalf("LoadConfig() = %#v", config)
	}
	for _, invalid := range []Config{
		{Enabled: true, Address: "", Port: 8001, StartTimeout: time.Second, ClientMaxBodySize: DefaultClientMaxBodySize},
		{Enabled: true, Address: "127.0.0.1", Port: 65536, StartTimeout: time.Second, ClientMaxBodySize: DefaultClientMaxBodySize},
		{Enabled: true, Address: "127.0.0.1", Port: 8001, ClientMaxBodySize: DefaultClientMaxBodySize},
		{Enabled: true, Address: "127.0.0.1", Port: 8001, StartTimeout: time.Second},
		{Enabled: true, Address: "127.0.0.1", Port: 8001, StartTimeout: time.Second, ClientMaxBodySize: DefaultClientMaxBodySize + 1},
	} {
		if err = invalid.Validate(); err == nil {
			t.Fatalf("Validate() error = nil for %#v", invalid)
		}
	}
}

func TestTransportAppliesClientMaxBodySize(t *testing.T) {
	config := DefaultConfig()
	config.ClientMaxBodySize = 1024
	transport, err := New(config, func(server *ghttp.Server) error {
		server.BindHandler("POST:/body", func(request *ghttp.Request) {
			content, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				request.Response.WriteStatus(stdhttp.StatusRequestEntityTooLarge)
				return
			}
			request.Response.Write(len(content))
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	server, listener := startServer(t, func(server *ghttp.Server) {
		if installErr := transport.install(server); installErr != nil {
			t.Fatal(installErr)
		}
	})
	t.Cleanup(func() { shutdownServer(t, server, listener) })

	for size, wantStatus := range map[int]int{
		1024: stdhttp.StatusOK,
		1025: stdhttp.StatusRequestEntityTooLarge,
	} {
		response, requestErr := stdhttp.Post(
			"http://"+listener.Addr().String()+"/body",
			"application/octet-stream",
			strings.NewReader(strings.Repeat("x", size)),
		)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != wantStatus {
			t.Fatalf("body size %d status = %d, want %d", size, response.StatusCode, wantStatus)
		}
	}
}

func TestPrepareReportsPortConflictAndPreparedStopReleasesPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := occupied.Addr().(*net.TCPAddr).Port
	transport := newHTTPTransport(t, port)
	if err = transport.Prepare(t.Context()); err == nil || !strings.Contains(err.Error(), "绑定 HTTP 监听地址失败") {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err = occupied.Close(); err != nil {
		t.Fatal(err)
	}
	transport = newHTTPTransport(t, port)
	if err = transport.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = transport.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	rebound, err := net.Listen("tcp", transport.listenAddress())
	if err != nil {
		t.Fatalf("listener was not released: %v", err)
	}
	_ = rebound.Close()
}

func TestReadyGateRejectsUntilApplicationReady(t *testing.T) {
	readiness := &readyProbe{}
	server, listener := startServer(t, func(server *ghttp.Server) {
		server.Use(readyGate(readiness))
		server.BindHandler("/ready", func(request *ghttp.Request) {
			request.Response.Write("ok")
		})
	})
	defer shutdownServer(t, server, listener)

	status, body := request(t, listener, "/ready")
	if status != stdhttp.StatusServiceUnavailable {
		t.Fatalf("unready response = (%d, %q)", status, body)
	}
	readiness.ready.Store(true)
	status, body = request(t, listener, "/ready")
	if status != stdhttp.StatusOK || body != "ok" {
		t.Fatalf("ready response = (%d, %q)", status, body)
	}
}

func TestTransportStartsWaitsForRunningAndStopsIdempotently(t *testing.T) {
	transport := newHTTPTransport(t, 0)
	if err := transport.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	terminated, err := transport.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	status, _ := request(t, transport.listener, "/ready")
	if status != stdhttp.StatusServiceUnavailable {
		t.Fatalf("response status = %d", status)
	}
	if err = transport.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = transport.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case _, open := <-terminated:
		if open {
			t.Fatal("termination channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("termination channel was not closed")
	}
}

func TestStartFailureAndTimeoutCloseOwnedListener(t *testing.T) {
	tests := []struct {
		name    string
		runtime *runtimeProbe
	}{
		{name: "start error", runtime: &runtimeProbe{startErr: errors.New("start failed")}},
		{name: "running timeout", runtime: &runtimeProbe{status: ghttp.ServerStatusStopped}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig()
			config.Address = "127.0.0.1"
			config.Port = 0
			config.StartTimeout = 20 * time.Millisecond
			transport := newTransport(config, func(*ghttp.Server) error { return nil }, func() serverRuntime {
				return test.runtime
			})
			if err := transport.Prepare(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !test.runtime.listenerSet.Load() {
				t.Fatal("Prepare() did not set listener")
			}
			address := test.runtime.listener.Addr().String()
			if _, err := transport.Start(t.Context()); err == nil {
				t.Fatal("Start() error = nil")
			}
			if test.name == "start error" && test.runtime.shutdownCalls.Load() != 0 {
				t.Fatal("synchronous Start failure called Shutdown")
			}
			rebound, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("listener was not released: %v", err)
			}
			_ = rebound.Close()
		})
	}
}

func TestStartTimeoutDoesNotWaitForShutdown(t *testing.T) {
	release := make(chan struct{})
	runtime := &runtimeProbe{status: ghttp.ServerStatusStopped, shutdown: release}
	config := DefaultConfig()
	config.Address = "127.0.0.1"
	config.Port = 0
	config.StartTimeout = 20 * time.Millisecond
	transport := newTransport(config, func(*ghttp.Server) error { return nil }, func() serverRuntime { return runtime })
	if err := transport.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := transport.Start(t.Context())
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Start() elapsed = %s", elapsed)
	}
}

func TestStopTreatsNetErrClosedAsSuccess(t *testing.T) {
	runtime := &runtimeProbe{status: ghttp.ServerStatusRunning, shutdownErr: net.ErrClosed}
	config := DefaultConfig()
	config.Address = "127.0.0.1"
	config.Port = 0
	transport := newTransport(config, func(*ghttp.Server) error { return nil }, func() serverRuntime { return runtime })
	if err := transport.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := transport.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestStopTimeoutStillClosesOwnedListener(t *testing.T) {
	release := make(chan struct{})
	runtime := &runtimeProbe{status: ghttp.ServerStatusRunning, shutdown: release}
	config := DefaultConfig()
	config.Address = "127.0.0.1"
	config.Port = 0
	transport := newTransport(config, func(*ghttp.Server) error { return nil }, func() serverRuntime { return runtime })
	if err := transport.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	address := runtime.listener.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := transport.Stop(ctx)
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v", err)
	}
	rebound, listenErr := net.Listen("tcp", address)
	if listenErr != nil {
		t.Fatalf("listener was not released: %v", listenErr)
	}
	_ = rebound.Close()
}

func TestGoFrameLifecycleContract(t *testing.T) {
	typeOfServer := reflect.TypeFor[*ghttp.Server]()
	start, exists := typeOfServer.MethodByName("Start")
	if !exists || start.Type.NumIn() != 1 || start.Type.NumOut() != 1 {
		t.Fatalf("ghttp.Server.Start signature = %v", start.Type)
	}
	shutdown, exists := typeOfServer.MethodByName("Shutdown")
	if !exists || shutdown.Type.NumIn() != 1 || shutdown.Type.NumOut() != 1 {
		t.Fatalf("ghttp.Server.Shutdown signature = %v", shutdown.Type)
	}
}

func TestServeFatalExitsProcess(t *testing.T) {
	if os.Getenv("COOL_HTTP_FATAL_HELPER") == "1" {
		transport := newHTTPTransport(t, 0)
		if err := transport.Prepare(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := transport.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := transport.listener.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestServeFatalExitsProcess$")
	command.Env = append(os.Environ(), "COOL_HTTP_FATAL_HELPER=1")
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("fatal helper error = %v", err)
	}
}

func newHTTPTransport(t *testing.T, port int) *Transport {
	t.Helper()
	config := DefaultConfig()
	config.Address = "127.0.0.1"
	config.Port = port
	transport, err := New(config, func(server *ghttp.Server) error {
		server.SetDumpRouterMap(false)
		server.SetAccessLogEnabled(false)
		server.SetErrorLogEnabled(false)
		server.BindHandler("/ready", func(request *ghttp.Request) {
			request.Response.Write("ok")
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return transport
}

func startServer(t *testing.T, configure func(*ghttp.Server)) (*ghttp.Server, net.Listener) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := ghttp.GetServer("app-http-test-" + time.Now().Format("150405.000000000"))
	server.SetDumpRouterMap(false)
	server.SetAccessLogEnabled(false)
	server.SetErrorLogEnabled(false)
	configure(server)
	if err = server.SetListener(listener); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	if err = server.Start(); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}

	return server, listener
}

func shutdownServer(t *testing.T, server *ghttp.Server, listener net.Listener) {
	t.Helper()
	if err := server.Shutdown(); err != nil {
		t.Error(err)
	}
	if err := closeListener(listener); err != nil {
		t.Error(err)
	}
}

func request(t *testing.T, listener net.Listener, path string) (int, string) {
	t.Helper()
	client := &stdhttp.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	return response.StatusCode, string(body)
}
