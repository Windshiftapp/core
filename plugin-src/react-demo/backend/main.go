package main

import (
	"fmt"
)

// This is a minimal WASM stub for the React demo plugin
// The plugin is primarily frontend-focused and doesn't require backend logic

func main() {
	fmt.Println("React Demo Plugin - Backend WASM initialized")
}

//export Init
func Init() int32 {
	return 0
}

//export HandleRequest
func HandleRequest(method, path, body string) string {
	return `{"status": "ok", "message": "React Demo Plugin"}`
}
