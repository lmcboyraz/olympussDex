package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cemilboyraz/oly2/internal/enginevectors"
)

func main() {
	output := flag.String(
		"output",
		"testdata/engine_vectors.json",
		"path for the tracked engine golden vectors",
	)
	flag.Parse()

	if err := enginevectors.Write(*output); err != nil {
		fmt.Fprintf(os.Stderr, "generate engine vectors: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", *output)
}
