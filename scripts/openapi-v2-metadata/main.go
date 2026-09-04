package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	v2 "windshift/internal/restapi/v2"
)

func main() {
	output := flag.String("out", "internal/restapi/v2/contract-metadata.json", "canonical route metadata output")
	check := flag.Bool("check", false, "fail when the checked metadata is stale")
	flag.Parse()

	paths := make(map[string]map[string]any)
	for _, route := range v2.Inventory() {
		path := paths[route.Path]
		if path == nil {
			path = make(map[string]any)
			paths[route.Path] = path
		}
		responses := make(map[string]any, len(route.DocumentedErrors))
		for _, status := range route.DocumentedErrors {
			responses[strconv.Itoa(status)] = map[string]any{}
		}
		path[strings.ToLower(route.Method)] = map[string]any{
			"tags":        []string{route.Tag},
			"summary":     route.Summary,
			"description": route.Description,
			"parameters":  route.Parameters,
			"responses":   responses,
		}
	}
	payload, err := json.MarshalIndent(map[string]any{"paths": paths}, "", "  ")
	if err != nil {
		fail("encode metadata: %v", err)
	}
	payload = append(payload, '\n')
	if *check {
		current, err := os.ReadFile(*output)
		if err != nil {
			fail("read metadata: %v", err)
		}
		if !bytes.Equal(current, payload) {
			fail("%s is stale", *output)
		}
		fmt.Printf("Canonical v2 route metadata is reproducible (%d operations).\n", len(v2.Inventory()))
		return
	}
	if err := os.WriteFile(*output, payload, 0o644); err != nil { //nolint:gosec // Packaging requires a world-readable generated artifact.
		fail("write metadata: %v", err)
	}
	fmt.Printf("Generated %s for %d canonical operations.\n", *output, len(v2.Inventory()))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "openapi-v2-metadata: "+format+"\n", args...)
	os.Exit(1)
}
