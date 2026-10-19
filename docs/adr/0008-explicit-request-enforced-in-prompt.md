# Explicit requests are enforced in the prompt

ADR 0001 requires an explicit request before a write tool runs. That rule is text in the tool-calling prompt. The executor does not detect whether the latest message asked for the change. After the required read has succeeded, a model call to the write tool becomes an HTTP call. A sentence classifier in the executor was rejected because "explicit" is not a schema check the worker can make reliably.
