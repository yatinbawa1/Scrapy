//go:build clip

package clip

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromTarball(t *testing.T) {
	tgz := os.Getenv("LOCAL_ORT_TGZ")
	if tgz == "" {
		t.Skip("set LOCAL_ORT_TGZ to a local onnxruntime .tgz to test extraction")
	}
	f, err := os.Open(tgz)
	if err != nil {
		t.Skipf("cannot open %s: %v", tgz, err)
	}
	defer f.Close()
	dst := filepath.Join(t.TempDir(), "libonnxruntime.dylib")
	if err := extractFromTarball(f, runtimeMember, dst); err != nil {
		t.Fatalf("extract: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat extracted lib: %v", err)
	}
	if info.Size() < 1_000_000 {
		t.Fatalf("extracted lib suspiciously small: %d bytes", info.Size())
	}
	t.Logf("extracted %s (%d bytes)", dst, info.Size())
}
