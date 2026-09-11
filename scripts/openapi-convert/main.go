// openapi-convert reads a Swagger 2.0 spec emitted by `swag init` and writes
// the equivalent OpenAPI 3.0 spec next to it. Run via `make openapi`.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
)

func main() {
	in := flag.String("in", "api/swagger.json", "path to swagger 2.0 input (json)")
	outJSON := flag.String("out-json", "api/openapi.json", "path to OpenAPI 3.0 json output")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		die("read %s: %v", *in, err)
	}

	var v2 openapi2.T
	if err := json.Unmarshal(data, &v2); err != nil {
		die("parse swagger 2.0: %v", err)
	}

	v3, err := openapi2conv.ToV3(&v2)
	if err != nil {
		die("convert to OpenAPI 3.0: %v", err)
	}

	// openapi2conv only emits a `servers` entry when the source spec set
	// `host`. swag doesn't emit one, so the converted doc has no servers
	// and routers like kin-openapi/legacy refuse to match anything. Inject
	// a relative-URL server using the basePath so request matching works.
	if len(v3.Servers) == 0 && v2.BasePath != "" {
		v3.AddServer(&openapi3.Server{URL: v2.BasePath})
	}
	if err := setOperationalServers(v3); err != nil {
		die("set operational servers: %v", err)
	}

	if err := v3.Validate(context.Background()); err != nil {
		die("validate OpenAPI 3.0: %v", err)
	}

	if err := writeJSON(*outJSON, v3); err != nil {
		die("write %s: %v", *outJSON, err)
	}

	fmt.Printf("converted %s -> %s\n", *in, *outJSON)
}

func setOperationalServers(doc *openapi3.T) error {
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		pathItem := doc.Paths.Find(path)
		if pathItem == nil {
			return fmt.Errorf("required path %s is missing", path)
		}
		pathItem.Servers = openapi3.Servers{{URL: "/"}}
	}
	return nil
}

func writeJSON(path string, doc *openapi3.T) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o600)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "openapi-convert: "+format+"\n", args...)
	os.Exit(1)
}
