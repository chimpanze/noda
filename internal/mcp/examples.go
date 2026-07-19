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
		"description": "Basic CRUD API with database operations. The users table must exist first — create it with a migration (see noda://docs/migrations). The migration_up/migration_down fields below are SQL (migrations/<timestamp>_create_users.up.sql / .down.sql), not Noda config. $ref note: {\"$ref\": \"schemas/User\"} resolves by definition name, never by filename#key — it works because a file under schemas/ defines a top-level \"User\" key (e.g. schemas/User.json = {\"User\": {...}}), or because schemas/User.json is itself a bare JSON Schema document (top level has \"type\"/\"properties\"), which registers under its filename.",
		// SQL migration files (not config). Filename: migrations/<YYYYMMDDHHMMSS>_create_users.up.sql / .down.sql
		"migration_up": `CREATE TABLE users (
  id         UUID PRIMARY KEY,
  name       TEXT NOT NULL,
  email      TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`,
		"migration_down": `DROP TABLE users;`,
		"route": `{
  "id": "users-crud",
  "method": "POST",
  "path": "/api/users",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "create-user",
    "input": {
      "name": "{{ body.name }}",
      "email": "{{ body.email }}"
    }
  }
}`,
		"workflow": `{
  "id": "create-user",
  "nodes": {
    "validate": {
      "type": "transform.validate",
      "config": {
        "schema": { "$ref": "schemas/User" }
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
		"description": "Authentication with the built-in auth plugin. Add `services.auth` = `{\"plugin\": \"auth\", \"config\": {\"database\": \"<your db service name>\"}}` to noda.json, then run `noda auth init` to scaffold the flows and the `auth_users` migration this plugin requires. The two workflows below (login, register) show the same shape those scaffolded flows use, built from the plugin's own nodes (`auth.verify_credentials`, `auth.create_session`, `auth.create_user`) — see docs/03-nodes/auth.*.md and docs/04-guides/authentication.md for the full node/output contracts and the anti-enumeration patterns `noda auth init` bakes in. An ALTERNATIVE hand-rolled variant is included below for reference only; the two are incompatible (#377) — pick one.",
		"service_config": `{
  "services": {
    "main-db": { "plugin": "postgres", "config": { "url": "{{ $env('DATABASE_URL') }}" } },
    "auth": { "plugin": "auth", "config": { "database": "main-db" } }
  }
}`,
		"route": `{
  "id": "login",
  "method": "POST",
  "path": "/api/auth/login",
  "trigger": {
    "workflow": "login",
    "input": {
      "email": "{{ body.email }}",
      "password": "{{ body.password }}"
    }
  }
}`,
		"workflow": `{
  "id": "login",
  "nodes": {
    "verify": {
      "type": "auth.verify_credentials",
      "services": { "auth": "auth", "database": "main-db" },
      "config": {
        "email": "{{ input.email }}",
        "password": "{{ input.password }}"
      }
    },
    "session": {
      "type": "auth.create_session",
      "services": { "auth": "auth", "database": "main-db" },
      "config": {
        "user_id": "{{ nodes.verify.id }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.session }}"
      }
    },
    "rejected": {
      "type": "response.error",
      "config": {
        "status": 401,
        "code": "INVALID_CREDENTIALS",
        "message": "invalid email or password"
      }
    }
  },
  "edges": [
    { "from": "verify", "to": "session", "output": "success" },
    { "from": "verify", "to": "rejected", "output": "invalid" },
    { "from": "session", "to": "respond", "output": "success" }
  ]
}`,
		"register_route": `{
  "id": "register",
  "method": "POST",
  "path": "/api/auth/register",
  "trigger": {
    "workflow": "register",
    "input": {
      "email": "{{ body.email }}",
      "password": "{{ body.password }}"
    }
  }
}`,
		"register_workflow": `{
  "id": "register",
  "nodes": {
    "create": {
      "type": "auth.create_user",
      "services": { "auth": "auth", "database": "main-db" },
      "config": {
        "email": "{{ input.email }}",
        "password": "{{ input.password }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.create }}"
      }
    },
    "conflict": {
      "type": "response.error",
      "config": {
        "status": 409,
        "code": "EMAIL_TAKEN",
        "message": "email already registered"
      }
    }
  },
  "edges": [
    { "from": "create", "to": "respond", "output": "success" },
    { "from": "create", "to": "conflict", "output": "exists" }
  ]
}`,
		"alternative_description": "ALTERNATIVE — hand-rolled JWT with your own users table, incompatible with the auth plugin's `auth_users` tables: choose one pattern (#377).",
		"alternative_root_config": `{
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
		"alternative_route": `{
  "id": "login",
  "method": "POST",
  "path": "/api/auth/login",
  "trigger": {
    "workflow": "login",
    "input": {
      "email": "{{ body.email }}",
      "password": "{{ body.password }}"
    }
  }
}`,
		"alternative_workflow": `{
  "id": "login",
  "nodes": {
    "lookup": {
      "type": "db.findOne",
      "config": {
        "table": "users",
        "where": { "email": "{{ input.email }}" }
      }
    },
    "check_user": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.lookup != nil }}"
      }
    },
    "verify": {
      "type": "transform.set",
      "config": {
        "fields": {
          "valid": "{{ bcrypt_verify(input.password, nodes.lookup.password_hash) }}"
        }
      }
    },
    "check_password": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.verify.valid == true }}"
      }
    },
    "sign_token": {
      "type": "util.jwt_sign",
      "config": {
        "claims": {
          "user_id": "{{ nodes.lookup.id }}",
          "email": "{{ nodes.lookup.email }}"
        },
        "secret": "{{ secrets.JWT_SECRET }}",
        "expiry": "24h"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "token": "{{ nodes.sign_token.token }}"
        }
      }
    },
    "not_found": {
      "type": "response.error",
      "config": {
        "status": 401,
        "message": "Invalid credentials"
      }
    },
    "wrong_password": {
      "type": "response.error",
      "config": {
        "status": 401,
        "message": "Invalid credentials"
      }
    }
  },
  "edges": [
    { "from": "lookup", "to": "check_user", "output": "success" },
    { "from": "check_user", "to": "verify", "output": "then" },
    { "from": "check_user", "to": "not_found", "output": "else" },
    { "from": "verify", "to": "check_password", "output": "success" },
    { "from": "check_password", "to": "sign_token", "output": "then" },
    { "from": "check_password", "to": "wrong_password", "output": "else" },
    { "from": "sign_token", "to": "respond", "output": "success" }
  ]
}`,
		"alternative_register_route": `{
  "id": "register",
  "method": "POST",
  "path": "/api/auth/register",
  "trigger": {
    "workflow": "register",
    "input": {
      "name": "{{ body.name }}",
      "email": "{{ body.email }}",
      "password": "{{ body.password }}"
    }
  }
}`,
		"alternative_register_workflow": `{
  "id": "register",
  "nodes": {
    "hash": {
      "type": "transform.set",
      "config": {
        "fields": {
          "password_hash": "{{ bcrypt_hash(input.password) }}"
        }
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
          "password_hash": "{{ nodes.hash.password_hash }}",
          "created_at": "{{ now() }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": {
          "id": "{{ nodes.create.id }}",
          "name": "{{ nodes.create.name }}",
          "email": "{{ nodes.create.email }}"
        }
      }
    }
  },
  "edges": [
    { "from": "hash", "to": "create", "output": "success" },
    { "from": "create", "to": "respond", "output": "success" }
  ]
}`,
	},
	"websocket": {
		"description": "WebSocket real-time broadcast: a POST persists a message and broadcasts it to subscribers of a channel. Handlers (on_connect/on_message/on_disconnect) are workflow-id STRINGS; see noda://docs/realtime for the subscription/channel/lifecycle model.",
		// noda.json snippet — a pubsub service is required so broadcasts fan out across instances (connections.sync.pubsub references it).
		"services": `{
  "services": {
    "events": {
      "plugin": "pubsub",
      "config": { "url": "{{ $env('REDIS_URL') }}" }
    }
  }
}`,
		// connections/*.json — endpoints is a map keyed by endpoint name; handlers are workflow ids.
		"connections": `{
  "sync": { "pubsub": "events" },
  "endpoints": {
    "board": {
      "type": "websocket",
      "path": "/ws/board/:room_id",
      "middleware": ["auth.jwt"],
      "channels": {
        "pattern": "board.{{ request.params.room_id }}",
        "max_per_channel": 100
      },
      "ping_interval": "30s",
      "on_message": "board-on-message"
    }
  }
}`,
		// routes/*.json — clients POST here; the workflow broadcasts to subscribers.
		"route": `{
  "id": "post-message",
  "method": "POST",
  "path": "/api/board/:room_id/messages",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "post-message",
    "input": {
      "room_id": "{{ params.room_id }}",
      "text": "{{ body.text }}"
    }
  }
}`,
		// workflows/*.json — ws.send binds the "connections" service slot to the endpoint name ("board").
		// Its "channel" must match the endpoint's channels.pattern for subscribers to receive it.
		"workflow": `{
  "id": "post-message",
  "nodes": {
    "broadcast": {
      "type": "ws.send",
      "services": { "connections": "board" },
      "config": {
        "channel": "board.{{ input.room_id }}",
        "data": { "text": "{{ input.text }}" }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": { "ok": true }
      }
    }
  },
  "edges": [
    { "from": "broadcast", "to": "respond", "output": "success" }
  ]
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
