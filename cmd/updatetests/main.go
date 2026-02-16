package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
)

func main() {
	// List of provider test files to update
	testFiles := []string{
		"provider/antigravity/antigravity_test.go",
		"provider/gemini/gemini_test.go",
		"provider/kiro/kiro_test.go",
		"provider/iflow/iflow_test.go",
		"provider/qwen/qwen_test.go",
	}

	for _, testFile := range testFiles {
		updateTestFile(testFile)
	}

	fmt.Println("All test files updated successfully")
}

func updateTestFile(filePath string) {
	// Read the file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", filePath, err)
		return
	}

	// List of all replacements to make
	replacements := []struct {
		old string
		new string
	}{
		{"\n\t\"github.com/sunbankio/qwencoder-proxy/auth\"\n", "\n\ttokpkg \"github.com/sunbankio/qwencoder-proxy/internal/token\"\n"},
		{"\bauth.", "\btokpkg."},
		{"*auth.ProxyConfig", "*tokpkg.ProxyConfig"},
		{"*auth.TokenManager", "*tokpkg.TokenManager"},
		{"*auth.NewProxyHealthTracker", "tokpkg.NewProxyHealthTracker"},
		{"*auth.NewStrategyFactory", "tokpkg.NewStrategyFactory"},
		{"*auth.NewEmailExtractionManager", "tokpkg.NewEmailExtractionManager"},
	}

	// Apply all replacements
	for _, repl := range replacements {
		content = bytes.ReplaceAll(content, []byte(repl.old), []byte(repl.new))
	}

	// Write back to file
	err = ioutil.WriteFile(filePath, content, 0644)
	if err != nil {
		fmt.Printf("Error writing file %s: %v\n", filePath, err)
		return
	}
}
