package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
)

func main() {
	// Read the file
	content, err := os.ReadFile("restapi/rest_api.go")
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Replace import statement
	content = bytes.Replace(content, []byte("github.com/sunbankio/qwencoder-proxy/auth\""), []byte("tokpkg \"github.com/sunbankio/qwencoder-proxy/internal/token\""), 1)

	// Replace all auth. references with tokpkg.
	re := regexp.MustCompile(`\bauth\.`)
	content = re.ReplaceAll(content, []byte("tokpkg."))

	// Write back to file
	err = os.WriteFile("restapi/rest_api.go", content, 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("File updated successfully")
}
