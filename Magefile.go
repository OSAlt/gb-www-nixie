//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/magefile/mage/mg"
)

// Default target to run when none is specified
var Default = Build

// Generate runs templ generation
func Generate() error {
	fmt.Println("Generating templ files...")
	cmd := exec.Command("templ", "generate")
	cmd.Dir = "."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Build compiles the application
func Build() error {
	mg.Deps(Generate)
	fmt.Println("Building...")
	cmd := exec.Command("go", "build", "-o", "bin/server", "./cmd/server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Run builds and runs the application
func Run() error {
	mg.Deps(Build)
	fmt.Println("Running...")
	cmd := exec.Command("./bin/server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning...")
	return os.RemoveAll("bin")
}
