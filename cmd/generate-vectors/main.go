package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/cemilboyraz/oly2/internal/vectors"
)

func main() {
	output := flag.String(
		"output",
		"testdata/protocol_vectors.json",
		"tracked golden-vector output path",
	)
	flag.Parse()

	if err := vectors.Write(*output); err != nil {
		log.Fatalf("generate vectors: %v", err)
	}
	fmt.Printf("wrote canonical protocol vectors: %s\n", *output)
}
