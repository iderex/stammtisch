package main

import "os"

// This file compiles and matches gofmt output, and carries a lint finding on
// purpose. It is never merged.
func deliberatelyLinted() {
	os.Remove("a-path-that-does-not-exist")
}
