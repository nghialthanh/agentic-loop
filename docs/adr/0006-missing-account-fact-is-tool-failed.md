# A missing requested account fact is `tool_failed`

`error` stays null when every account fact the latest message asked for was returned, or when the message asked for none. When a tool fails inside the message budget and a requested account fact is therefore missing, `error.code` is `tool_failed`, including when the reply honestly says so and when other account facts in the reply are grounded. Leaving `error` null on an honest failure was rejected because the legacy chat would treat that reply as a completed answer.
