//go:build clip

package clip

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// modelAssets maps the local filename to its download URL. The CLIP weights are
// the inference4j exports of openai/clip-vit-base-patch32; the ONNX Runtime
// native library is matched to the onnxruntime_go binding version (1.27.0) and
// ships inside a .tgz archive (see runtimeMember for the path within it).
var modelAssets = []struct {
	name string
	url  string
}{
	{"vocab.json", "https://huggingface.co/inference4j/clip-vit-base-patch32/resolve/main/vocab.json"},
	{"merges.txt", "https://huggingface.co/inference4j/clip-vit-base-patch32/resolve/main/merges.txt"},
	{"vision_model.onnx", "https://huggingface.co/inference4j/clip-vit-base-patch32/resolve/main/vision_model.onnx"},
	{"text_model.onnx", "https://huggingface.co/inference4j/clip-vit-base-patch32/resolve/main/text_model.onnx"},
	{"libonnxruntime.dylib", "https://github.com/microsoft/onnxruntime/releases/download/v1.27.0/onnxruntime-osx-arm64-1.27.0.tgz"},
}

// runtimeMember is the path of the native library inside the ONNX Runtime
// tarball (matched by the asset whose name is the .dylib).
const runtimeMember = "onnxruntime-osx-arm64-1.27.0/lib/libonnxruntime.dylib"

// EnsureModels downloads any missing CLIP model/runtime files into modelDir.
// It is a no-op (nil error) once everything is present, and never overwrites
// existing files. Errors are returned so the caller can fall back gracefully.
func EnsureModels(modelDir string) error {
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return fmt.Errorf("creating model dir: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	for _, a := range modelAssets {
		dst := filepath.Join(modelDir, a.name)
		if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
			continue
		}
		if err := download(client, a.url, dst); err != nil {
			return fmt.Errorf("downloading %s: %w", a.name, err)
		}
	}
	return nil
}

func download(client *http.Client, url, dst string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	if strings.HasSuffix(url, ".tgz") || strings.HasSuffix(url, ".tar.gz") {
		return extractFromTarball(resp.Body, runtimeMember, dst)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// extractFromTarball writes the single member at memberPath out of a gzipped
// tar stream to dst.
func extractFromTarball(r io.Reader, memberPath, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == memberPath {
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
			found = true
			break
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if !found {
		os.Remove(tmp)
		return fmt.Errorf("member %s not found in archive", memberPath)
	}
	return os.Rename(tmp, dst)
}
