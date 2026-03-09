package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [project-name]",
		Short: "Initialize a new Noda project",
		Long:  "Scaffold a new Noda project with config files, Docker Compose, and sample routes/workflows.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			return scaffoldProject(name)
		},
	}
	return cmd
}

func scaffoldProject(name string) error {
	// Create project directory
	if err := os.MkdirAll(name, 0755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	dirs := []string{
		"routes",
		"workflows",
		"schemas",
		"tests",
		"migrations",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(name, d), 0755); err != nil {
			return fmt.Errorf("create %s directory: %w", d, err)
		}
	}

	files := map[string]string{
		"noda.json":             nodaJSON,
		".env.example":          envExample,
		"docker-compose.yml":    dockerCompose,
		"routes/api.json":       sampleRoute,
		"workflows/hello.json":  sampleWorkflow,
		"schemas/greeting.json": sampleSchema,
		"tests/hello.test.json": sampleTest,
		"README.md":             readmeTemplate(name),
	}

	for path, content := range files {
		fullPath := filepath.Join(name, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	fmt.Printf("✓ Project %q created\n", name)
	fmt.Println()
	fmt.Println("  Get started:")
	fmt.Printf("    cd %s\n", name)
	fmt.Println("    cp .env.example .env")
	fmt.Println("    docker compose up -d")
	fmt.Println("    noda dev")
	fmt.Println()
	return nil
}

const nodaJSON = `{
  "server": {
    "port": 3000,
    "read_timeout": "30s",
    "write_timeout": "30s",
    "body_limit": 5242880
  }
}
`

const envExample = `# Database
DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379/0

# JWT
JWT_SECRET=change-me-in-production
`

const dockerCompose = `services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: noda
      POSTGRES_PASSWORD: noda
      POSTGRES_DB: noda
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  pgdata:
`

const sampleRoute = `{
  "id": "hello-route",
  "method": "GET",
  "path": "/api/hello/:name",
  "trigger": {
    "workflow": "hello",
    "input": {
      "name": "{{ request.params.name }}"
    }
  }
}
`

const sampleWorkflow = `{
  "id": "hello",
  "nodes": {
    "greet": {
      "type": "transform.set",
      "config": {
        "fields": {
          "message": "Hello, {{ input.name }}!"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "greeting": "{{ nodes.greet.message }}"
        }
      }
    }
  },
  "edges": [
    { "from": "greet", "to": "respond", "output": "success" }
  ]
}
`

const sampleSchema = `{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "minLength": 1,
      "maxLength": 100
    }
  },
  "required": ["name"]
}
`

const sampleTest = `{
  "id": "hello-test",
  "workflow": "hello",
  "tests": [
    {
      "name": "greets by name",
      "input": { "name": "World" },
      "expect": {
        "status": "success",
        "output": {
          "greeting": "Hello, World!"
        }
      }
    }
  ]
}
`

func readmeTemplate(name string) string {
	return fmt.Sprintf(`# %s

A [Noda](https://github.com/chimpanze/noda) project.

## Getting Started

`+"```"+`bash
# Start infrastructure
cp .env.example .env
docker compose up -d

# Run in development mode
noda dev

# Run tests
noda test

# Validate config
noda validate --verbose
`+"```"+`

## Project Structure

`+"```"+`
noda.json           — main configuration (server, services, security)
routes/             — HTTP route definitions
workflows/          — workflow definitions
schemas/            — JSON schemas for validation
tests/              — workflow test suites
migrations/         — database migrations
docker-compose.yml  — local infrastructure
`+"```"+`
`, name)
}
