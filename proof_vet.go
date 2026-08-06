package main

import "fmt"

// This file compiles and carries a vet diagnostic on purpose. It is never
// merged.
func deliberatelyUnvetted() {
	fmt.Printf("%d\n")
}
