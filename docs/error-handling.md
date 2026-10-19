# Error-handling matrix

How the worker maps failure modes to Kafka output, DLQ, and `error.code`.

| Failure | Outcome | `error.code` | Notes |
| --- | --- | --- | --- |
| Malformed input (schema, empty content, unknown history role) | DLQ only | — | Original payload + reason. No claim, no normal output. |
| Customer over rate limit | Output | `rate_limited` | Fixed reply; no model or tools. Winner only counts. |
| Input guard block | Output | `guard_blocked` | Fixed refusal; no model or tools. |
| No skill match (empty `skill_names` or selection JSON decode failure) | Output | `null` | General reply; empty `skills` / `used_tools`; low confidence. |
| Selection / tool-calling model failure after claim | Output | `no_reply` | Short “can't answer” reply; model field set when the model was called. |
| Message timeout budget spent (wall-clock from attempt start) | Output | `timeout` | Short timeout reply; stops the loop. |
| Tool HTTP / keep normalization failure with missing requested account fact | Output | `tool_failed` | Customer-safe wording; no guessed account facts. |
| Unmet `requires` / invalid tool args | Output (continuing) | — (tool row `status=error`) | No HTTP on unmet requires or schema-invalid args; unmet requires do not count toward the four-execution cap. |
| Output fails schema validation before publish | Refuse publish | — | Offset not committed; message can redeliver. |

Successful grounded replies publish with `error: null`. Hallucination detection still publishes; it increments the hallucination metric when the reply asserted keep-field values no successful tool result contained.
