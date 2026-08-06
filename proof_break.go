package main

// This file exists to red the build check on purpose. It is never merged.
func deliberatelyBroken() int {
	var n int = "this is not an int"
	return n
}
