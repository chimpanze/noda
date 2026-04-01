# util.uuid

Generates a UUID v4. No configuration required.

## Config

No configuration fields.

## Outputs

`success`, `error`

Output is the UUID string.

## Behavior

Generates a random UUID v4 string. Fires `success` with the UUID as output data.

## Example

```json
{
  "type": "util.uuid",
  "config": {}
}
```

### With data flow

Generate a unique ID and use it as a key when inserting a new record.

```json
{
  "gen_id": {
    "type": "util.uuid",
    "config": {}
  }
}
```

Output stored as `nodes.gen_id`:
```json
"f47ac10b-58cc-4372-a567-0e02b2c3d479"
```

A downstream node can reference this value:
```json
{
  "create_record": {
    "type": "db.insert",
    "config": {
      "table": "documents",
      "data": {
        "id": "{{ nodes.gen_id }}",
        "title": "{{ input.title }}"
      }
    }
  }
}
```
