package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var examplePatterns = map[string]map[string]string{
	"crud": {
		"description": "Basic CRUD API with database operations",
		"route": `{
  "id": "users-crud",
  "method": "POST",
  "path": "/api/users",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "create-user",
    "input": {
      "name": "{{ request.body.name }}",
      "email": "{{ request.body.email }}"
    }
  }
}`,
		"workflow": `{
  "id": "create-user",
  "nodes": {
    "validate": {
      "type": "transform.validate",
      "config": {
        "schema": "$ref(schemas/user.json)"
      }
    },
    "create": {
      "type": "db.create",
      "config": {
        "table": "users",
        "data": {
          "id": "{{ $uuid() }}",
          "name": "{{ input.name }}",
          "email": "{{ input.email }}",
          "created_at": "{{ now() }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.create }}"
      }
    },
    "error": {
      "type": "response.error",
      "config": {
        "status": 400,
        "message": "Validation failed"
      }
    }
  },
  "edges": [
    { "from": "validate", "to": "create", "output": "success" },
    { "from": "validate", "to": "error", "output": "error" },
    { "from": "create", "to": "respond", "output": "success" }
  ]
}`,
	},
	"auth": {
		"description": "JWT authentication with login endpoint",
		"root_config": `{
  "security": {
    "jwt": {
      "secret": "{{ $env('JWT_SECRET') }}",
      "algorithm": "HS256",
      "token_lookup": "header:Authorization"
    }
  },
  "middleware_presets": {
    "authenticated": ["auth.jwt"],
    "public": []
  }
}`,
		"route": `{
  "id": "login",
  "method": "POST",
  "path": "/api/auth/login",
  "trigger": {
    "workflow": "login",
    "input": {
      "email": "{{ request.body.email }}",
      "password": "{{ request.body.password }}"
    }
  }
}`,
	},
	"websocket": {
		"description": "WebSocket real-time connection",
		"connection": `{
  "id": "live-updates",
  "type": "websocket",
  "path": "/ws/updates",
  "middleware": ["auth.jwt"],
  "on_connect": {
    "workflow": "ws-connect",
    "input": {
      "user_id": "{{ auth.user_id }}"
    }
  },
  "on_message": {
    "workflow": "ws-message",
    "input": {
      "user_id": "{{ auth.user_id }}",
      "data": "{{ message }}"
    }
  }
}`,
	},
	"file-upload": {
		"description": "File upload handling with storage",
		"route": `{
  "id": "upload-file",
  "method": "POST",
  "path": "/api/files/upload",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "handle-upload",
    "input": {}
  }
}`,
		"workflow": `{
  "id": "handle-upload",
  "nodes": {
    "upload": {
      "type": "upload.handle",
      "config": {
        "field": "file",
        "max_size": 10485760,
        "allowed_types": ["image/jpeg", "image/png", "application/pdf"]
      }
    },
    "store": {
      "type": "storage.write",
      "config": {
        "path": "uploads/{{ $uuid() }}/{{ nodes.upload.filename }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "path": "{{ nodes.store.path }}",
          "size": "{{ nodes.upload.size }}"
        }
      }
    }
  },
  "edges": [
    { "from": "upload", "to": "store", "output": "success" },
    { "from": "store", "to": "respond", "output": "success" }
  ]
}`,
	},
	"scheduled-job": {
		"description": "Scheduled/cron job configuration",
		"schedule": `{
  "id": "daily-cleanup",
  "cron": "0 3 * * *",
  "timezone": "UTC",
  "workflow": "cleanup-expired",
  "input": {
    "days_old": 30
  }
}`,
		"workflow": `{
  "id": "cleanup-expired",
  "nodes": {
    "delete": {
      "type": "db.exec",
      "config": {
        "query": "DELETE FROM sessions WHERE created_at < NOW() - INTERVAL '{{ input.days_old }} days'"
      }
    },
    "log": {
      "type": "util.log",
      "config": {
        "level": "info",
        "message": "Cleaned up expired sessions"
      }
    }
  },
  "edges": [
    { "from": "delete", "to": "log", "output": "success" }
  ]
}`,
	},
}

func getExamplesHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pattern := req.GetString("pattern", "all")

	if pattern == "all" {
		result := make(map[string]any, len(examplePatterns))
		for name, example := range examplePatterns {
			result[name] = example
		}
		return jsonResult(map[string]any{
			"patterns":  result,
			"available": sortedMapKeys(examplePatterns),
		})
	}

	example, ok := examplePatterns[pattern]
	if !ok {
		available := sortedMapKeys(examplePatterns)
		return mcp.NewToolResultError(fmt.Sprintf("unknown pattern %q, available: %s",
			pattern, strings.Join(available, ", "))), nil
	}

	return jsonResult(map[string]any{
		"pattern": pattern,
		"configs": example,
	})
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
