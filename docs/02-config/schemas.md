# Schemas

Files in `schemas/*.json`. Each file contains named JSON Schema definitions.

```json
{
  "Task": {
    "type": "object",
    "properties": {
      "id": { "type": "integer" },
      "title": { "type": "string" },
      "completed": { "type": "boolean" }
    }
  },
  "CreateTask": {
    "type": "object",
    "properties": {
      "title": { "type": "string", "minLength": 1 }
    },
    "required": ["title"]
  }
}
```

Referenced from routes and nodes with `$ref`:

```json
{ "$ref": "schemas/Task#CreateTask" }
```

## How `$ref` Resolution Works

Schema references use the format `schemas/<filename_without_extension>#<key>`. Given a file `schemas/Task.json` containing keys `Task` and `CreateTask`, the available refs are:

- `schemas/Task#Task`
- `schemas/Task#CreateTask`

For schemas in subdirectories (e.g. `schemas/validation/User.json` with key `CreateUser`), the ref is `schemas/validation/User#CreateUser`.

During config loading, all `$ref` values are resolved and inlined before workflows or routes run. This means refs work in routes, workflows, workers, schedules, and connections -- anywhere a schema is expected.

## Request Body Validation

Define a `body.schema` on a route to validate incoming request bodies before the workflow runs. Set `body.validate` to `true` to enable validation:

```json
{
  "id": "create-user",
  "method": "POST",
  "path": "/api/users",
  "body": {
    "validate": true,
    "schema": { "$ref": "schemas/User#CreateUser" }
  },
  "trigger": {
    "workflow": "create-user",
    "input": {
      "name": "{{ body.name }}",
      "email": "{{ body.email }}"
    }
  }
}
```

### Schema Examples

**String constraints:**

```json
{
  "CreateUser": {
    "type": "object",
    "properties": {
      "name": {
        "type": "string",
        "minLength": 1,
        "maxLength": 100
      },
      "email": {
        "type": "string",
        "format": "email"
      },
      "role": {
        "type": "string",
        "enum": ["admin", "editor", "viewer"]
      },
      "bio": {
        "type": "string",
        "maxLength": 500
      }
    },
    "required": ["name", "email", "role"]
  }
}
```

**Nested objects:**

```json
{
  "CreateOrder": {
    "type": "object",
    "properties": {
      "customer_id": { "type": "string" },
      "shipping_address": {
        "type": "object",
        "properties": {
          "street": { "type": "string", "minLength": 1 },
          "city": { "type": "string", "minLength": 1 },
          "country": { "type": "string", "minLength": 2, "maxLength": 2 },
          "postal_code": { "type": "string" }
        },
        "required": ["street", "city", "country"]
      },
      "items": {
        "type": "array",
        "minItems": 1,
        "items": {
          "type": "object",
          "properties": {
            "product_id": { "type": "string" },
            "quantity": { "type": "integer", "minimum": 1 }
          },
          "required": ["product_id", "quantity"]
        }
      }
    },
    "required": ["customer_id", "shipping_address", "items"]
  }
}
```

**Numeric constraints:**

```json
{
  "UpdateInventory": {
    "type": "object",
    "properties": {
      "quantity": { "type": "integer", "minimum": 0, "maximum": 99999 },
      "price": { "type": "number", "minimum": 0, "exclusiveMinimum": 0 },
      "discount_percent": { "type": "number", "minimum": 0, "maximum": 100 }
    },
    "required": ["quantity", "price"]
  }
}
```

## Query Parameter Validation

Define a `query.schema` on a route to validate query string parameters. Query values arrive as strings, so use `"type": "string"` for individual fields:

```json
{
  "id": "list-users",
  "method": "GET",
  "path": "/api/users",
  "query": {
    "schema": {
      "type": "object",
      "properties": {
        "page": { "type": "string", "pattern": "^[0-9]+$" },
        "per_page": { "type": "string", "pattern": "^[0-9]+$" },
        "sort": { "type": "string", "enum": ["name", "created_at", "email"] },
        "order": { "type": "string", "enum": ["asc", "desc"] }
      }
    }
  },
  "trigger": {
    "workflow": "list-users",
    "input": {
      "page": "{{ query.page }}",
      "per_page": "{{ query.per_page }}",
      "sort": "{{ query.sort }}",
      "order": "{{ query.order }}"
    }
  }
}
```

## Path Parameter Validation

Define a `params.schema` on a route to validate URL path parameters:

```json
{
  "id": "get-user",
  "method": "GET",
  "path": "/api/users/:id",
  "params": {
    "schema": {
      "type": "object",
      "properties": {
        "id": { "type": "string", "pattern": "^[0-9a-f-]{36}$" }
      },
      "required": ["id"]
    }
  },
  "trigger": {
    "workflow": "get-user",
    "input": {
      "user_id": "{{ params.id }}"
    }
  }
}
```

## Reusable Schema Definitions with `$ref`

Define schemas once in `schemas/` files and reference them across multiple routes. This avoids duplication and keeps validation rules consistent:

**schemas/Pagination.json**

```json
{
  "PaginationQuery": {
    "type": "object",
    "properties": {
      "page": { "type": "string", "pattern": "^[0-9]+$" },
      "per_page": { "type": "string", "pattern": "^[0-9]+$" }
    }
  }
}
```

**routes/users.json** (referencing the shared schema)

```json
{
  "id": "list-users",
  "method": "GET",
  "path": "/api/users",
  "query": {
    "schema": { "$ref": "schemas/Pagination#PaginationQuery" }
  },
  "trigger": {
    "workflow": "list-users",
    "input": {
      "page": "{{ query.page }}",
      "per_page": "{{ query.per_page }}"
    }
  }
}
```

The same `PaginationQuery` schema can be referenced from any route that supports pagination.

## Response Validation

Define per-status-code schemas under a `response` key on a route to validate outgoing response bodies. The `response.validate` field controls the validation mode:

| Mode | Behavior |
|------|----------|
| (not set) | Validates only when dev mode is active; logs warnings |
| `"warn"` | Always validates; logs warnings but sends the response as-is |
| `"strict"` | Always validates; returns a 500 `RESPONSE_VALIDATION_ERROR` if the response body does not match the schema |

```json
{
  "id": "get-user",
  "method": "GET",
  "path": "/api/users/:id",
  "trigger": {
    "workflow": "get-user",
    "input": { "user_id": "{{ params.id }}" }
  },
  "response": {
    "validate": "warn",
    "200": {
      "description": "User found",
      "schema": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "name": { "type": "string" },
          "email": { "type": "string", "format": "email" },
          "created_at": { "type": "string" }
        },
        "required": ["id", "name", "email"]
      }
    }
  }
}
```

Response validation is useful for catching regressions -- if a workflow accidentally omits a required field, `"warn"` mode logs it and `"strict"` mode blocks the invalid response from reaching the client.

## How Validation Errors Are Returned

When request validation fails (body, query, or params), the server returns a structured JSON error response with field-level details.

**Body validation failure (HTTP 422):**

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request body validation failed",
    "details": {
      "errors": [
        { "field": "/name", "message": "missing property" },
        { "field": "/email", "message": "\"not-an-email\" is not valid \"email\"" }
      ]
    },
    "trace_id": "abc-123"
  }
}
```

**Query/params validation failure (HTTP 400):**

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Query parameter validation failed",
    "details": {
      "errors": [
        { "field": "/sort", "message": "value must be one of \"name\", \"created_at\", \"email\"" }
      ]
    },
    "trace_id": "abc-123"
  }
}
```

The `field` uses JSON Pointer syntax (e.g. `/shipping_address/city` for a nested field).

## Integration with `transform.validate`

The `transform.validate` node validates data inside a workflow using the same JSON Schema engine. This is useful for validating WebSocket messages, worker inputs, or data from external APIs -- cases where route-level validation does not apply.

```json
{
  "validate_input": {
    "type": "transform.validate",
    "config": {
      "data": "{{ input.data }}",
      "schema": { "$ref": "schemas/EditOperation#EditOperation" }
    }
  }
}
```

The node has two output ports:

- `success` -- the validated data passes through unchanged.
- `error` -- validation failed; the error output contains field-level details in the same format as route-level validation.

Use edges to handle validation failures gracefully:

```json
{
  "edges": [
    { "from": "validate_input", "to": "process", "output": "success" },
    { "from": "validate_input", "to": "send_error", "output": "error" }
  ]
}
```
