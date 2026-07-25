package enginevectors

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTrackedGoldenVectorsAreCurrent(t *testing.T) {
	generated, err := Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "testdata", "engine_vectors.json")
	tracked, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tracked vectors: %v", err)
	}
	if !bytes.Equal(tracked, generated) {
		t.Fatalf(
			"%s is stale; inspect the semantic change, then run make generate-engine-vectors",
			path,
		)
	}
}
