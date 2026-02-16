package main

import (
	"os"
	"regexp"
)

func main() {
	// Read the file
	content, err := os.ReadFile("restapi/rest_api.go")
	if err != nil {
		panic(err)
	}

	// Replace all token. references with tokpkg. (except for the import line)
	re := regexp.MustCompile(`\btokpkg\.`)
	content = re.ReplaceAll(content, []byte("tokpkg."))

	// Write back to file
	err = os.WriteFile("restapi/rest_api.go", content, 0644)
	if err != nil {
		panic(err)
	}

	println("File updated successfully")
}
