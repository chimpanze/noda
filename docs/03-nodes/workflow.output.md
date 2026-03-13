# workflow.output

Terminal node that declares a named output for the workflow. Used in sub-workflows called by `workflow.run`.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Output name |
| `data` | any (expr) | no | Output data |

## Outputs

None (terminal node).

## Behavior

When reached, the sub-workflow completes with this output name and the resolved `data`. The parent's `workflow.run` node fires the output matching this `name`.

All `workflow.output` nodes in a sub-workflow must have unique `name` values. They must be on mutually exclusive branches (validated at startup).

## Example

```json
{
  "type": "workflow.output",
  "config": {
    "name": "created",
    "data": "{{ nodes.insert }}"
  }
}
```
