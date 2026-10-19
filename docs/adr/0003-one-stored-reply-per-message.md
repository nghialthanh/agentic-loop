# One stored reply per message

A duplicate delivery must not call a write tool again and must not publish a second reply. The worker claims the message id before any tool call, stores the output record, then publishes that stored record. A redelivery publishes the stored record and does not call tools. Another worker backs off while the claim is live and may take over only after the claim expires. Publish-then-remember was rejected because a crash between the publish and the record runs the write a second time.
