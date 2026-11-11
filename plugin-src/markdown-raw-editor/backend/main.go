package main

import "fmt"

// The raw Markdown editor is frontend-only. The plugin system still requires a
// WASM entry point, so this is a minimal stub with no exports: the host falls
// back to the manifest extensions when get_metadata/get_routes are absent.
func main() {
	fmt.Println("markdown-raw-editor: backend stub initialized")
}
