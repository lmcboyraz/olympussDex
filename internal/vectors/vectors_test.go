package vectors

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTrackedGoldenVectorsMatchCanonicalImplementation(t *testing.T) {
	expected, err := Marshal()
	if err != nil {
		t.Fatalf("build expected vectors: %v", err)
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate vectors test")
	}
	goldenPath := filepath.Join(
		filepath.Dir(currentFile),
		"..",
		"..",
		"testdata",
		"protocol_vectors.json",
	)
	actual, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read tracked vectors (run make generate-vectors intentionally): %v", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("tracked vectors are stale; inspect the protocol change, then run make generate-vectors")
	}
}
