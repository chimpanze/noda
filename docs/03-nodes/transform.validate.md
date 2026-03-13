# transform.validate

Validates data against a JSON Schema. Use this for in-workflow validation of intermediate data or computed values. For request body validation, prefer defining `body.schema` on the route config -- it validates automatically before the workflow runs.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `data` | string (expr) | no | Data to validate (default: `{{ input }}`) |
| `schema` | object | yes | JSON Schema definition |

## Outputs

`success`, `error`

On validation failure, routes to the `error` output with a `ValidationError` containing field-level details.

## Behavior

Resolves `data`. Validates it against `schema`. If valid, fires `success` with the data unchanged. If invalid, fires `error` with a `ValidationError` containing field-level details.

## Example

```json
{
  "type": "transform.validate",
  "config": {
    "schema": {
      "type": "object",
      "properties": {
        "email": { "type": "string", "format": "email" },
        "age": { "type": "integer", "minimum": 18 }
      },
      "required": ["email"]
    }
  }
}
```
