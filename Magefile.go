//go:build mage

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/joho/godotenv"

	"github.com/magefile/mage/mg"
)

// Default target to run when none is specified
var Default = Build

// Generate runs templ and sqlc generation
func Generate() error {
	fmt.Println("Generating templ files...")
	cmd := exec.Command("go", "run", "-modfile", "tools/go.mod", "github.com/a-h/templ/cmd/templ", "generate")
	cmd.Dir = "."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	if err := DumpSchema(); err != nil {
		return err
	}

	fmt.Println("Generating sqlc files...")
	cmd = exec.Command("go", "run", "-modfile", "tools/go.mod", "github.com/sqlc-dev/sqlc/cmd/sqlc", "generate")
	cmd.Dir = "."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DumpSchema dumps the database schema to db/schema.sql
func DumpSchema() error {
	readEnv()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	fmt.Println("Dumping schema to db/schema.sql...")

	// We'll use docker exec to run pg_dump since the DB is in a container
	// If DATABASE_URL is localhost, we might need a different approach, but based on docker-compose.dev.yaml it should work.
	// Actually, let's try to parse the DSN to get db name and user, or just use the container name from docker-compose.
	// The container name is usually project_service_1.

	// Better approach: use pg_dump directly if available, or via docker if we know the container name.
	// Based on previous output, the container is newnixie-db-1.

	cmd := exec.Command("docker", "compose", "-f", "docker-compose.dev.yaml", "exec", "db", "pg_dump", "-U", "nixie", "-d", "nixiedb", "--schema-only", "--no-owner", "--no-privileges")

	// Filter out \restrict and \unrestrict lines that sqlc doesn't like
	// and also some other postgres-specific commands that might cause issues.
	stdout, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(stdout), "\n")
	var filteredLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "\\") {
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	return os.WriteFile("db/schema.sql", []byte(strings.Join(filteredLines, "\n")), 0644)
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

// Goose runs database migrations using goose with provided arguments
func Goose(arg string) error {
	readEnv()
	fmt.Println("Running goose...")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	args := []string{"run", "-modfile", "tools/go.mod", "github.com/pressly/goose/v3/cmd/goose", "-dir", "db/migrations", "postgres", dsn}
	// Split by space to support multiple arguments in one string
	args = append(args, strings.Fields(arg)...)

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Migrate runs database migrations using goose up
func Migrate() error {
	migErr := Goose("up")
	DumpSchema()
	return migErr
}

// MigrateCreate creates a new migration file
func MigrateCreate(name string) error {
	readEnv()
	fmt.Printf("Creating migration: %s\n", name)

	args := []string{"run", "-modfile", "tools/go.mod", "github.com/pressly/goose/v3/cmd/goose", "-dir", "db/migrations", "create", name, "sql"}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DbUp starts the database container for development
func DbUp() error {
	fmt.Println("Starting database...")
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.dev.yaml", "up", "-d", "db")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DbDown stops the database container for development
func DbDown() error {
	fmt.Println("Stopping database...")
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.dev.yaml", "stop", "db")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Dev runs the database and the application in development mode
func Dev() {
	mg.SerialDeps(DbUp, Migrate, Run)
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning...")
	return os.RemoveAll("bin")
}

// Docker builds the docker image
func Docker() error {
	fmt.Println("Building Docker image...")
	cmd := exec.Command("docker", "build", "-t", "nixiesite:latest", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func readEnv() {
	const envFile = ".env"
	err := godotenv.Load(envFile)
	if err == nil {
		slog.Info("local .env file loaded")
	}
}

func Init() {
	readEnv()
}
