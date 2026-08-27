package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestBinarySmoke builds integra-web, boots it in mock mode on a free
// loopback port, and asserts the HTTP surface responds. This guards the
// release artifact: if the binary can't start and serve, the release fails.
func TestBinarySmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in -short mode")
	}

	repoRoot := findModuleRoot(t)
	bin := filepath.Join(t.TempDir(), "integra-web")
	build := exec.Command("go", "build", "-o", bin, "./cmd/integra-web")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	addr := freeAddr(t)
	cmd := exec.Command(bin)
	cmd.Dir = repoRoot // so findStatic() resolves static/ or Integra/static
	cmd.Env = append(os.Environ(),
		"INTEGRA_MOCK=1",
		"INTEGRA_HTTP_ADDR="+addr,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start binary: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	url := "http://" + addr + "/healthz"
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 && string(body) == "ok" {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("healthz: status %d body %q", resp.StatusCode, body)
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("healthz never became healthy: %v", lastErr)
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || len(body) == 0 {
		t.Fatalf("GET /: status %d len %d", resp.StatusCode, len(body))
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Test runs in cmd/integra-web; module root is two levels up.
	dir := filepath.Dir(filepath.Dir(wd))
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("module root with go.mod not found from %s", wd)
	}
	return dir
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}
