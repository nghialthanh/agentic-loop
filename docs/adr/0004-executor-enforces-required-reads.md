# The executor enforces required reads

A write tool such as refunding an order must not run until its required read has succeeded in the same attempt. The tool row lists those tool names. The executor refuses the call when they have not succeeded, records the tool as failed, and makes no HTTP request. Prompt-only enforcement was rejected because the model can skip the read.
