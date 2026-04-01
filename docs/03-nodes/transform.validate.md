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

### With data flow

A workflow collects computed values from multiple nodes and validates them before inserting into the database.

```json
{
  "validate_order": {
    "type": "transform.validate",
    "config": {
      "data": "{{ nodes.build_order }}",
      "schema": {
        "type": "object",
        "properties": {
          "product_id": { "type": "integer" },
          "quantity": { "type": "integer", "minimum": 1 },
          "shipping_address": { "type": "string", "minLength": 5 }
        },
        "required": ["product_id", "quantity", "shipping_address"]
      }
    }
  }
}
```

Output stored as `nodes.validate_order` (on success, the data passes through unchanged):
```json
{
  "product_id": 42,
  "quantity": 3,
  "shipping_address": "123 Main St, Springfield"
}
```

Downstream nodes access fields via `nodes.validate_order.product_id`. On validation failure, the `error` output fires with a `ValidationError` containing field-level details.
