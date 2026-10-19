# Customer support worker

The worker that reads one customer message and publishes one reply. The legacy chat owns the conversation.

## Speakers

**Worker**:
The process that handles one message at a time and publishes one reply.
_Avoid_: agent, bot

**Model**:
The language model the worker calls.
_Avoid_: agent, worker

**Customer**:
The person the legacy chat identifies with a customer id.
_Avoid_: user, account

**Agent**:
A role on a history turn. That turn is a prior reply in the thread, from the worker or from a human in the legacy chat.
_Avoid_: worker, model

## Catalog

**Skill**:
A named group of tools for one kind of customer request.
_Avoid_: intent, plugin

**Intent**:
The short label for what the latest message is asking.
_Avoid_: skill

**Tool**:
A single operation the worker can call for one skill.
_Avoid_: skill

**Read tool**:
A tool that returns account facts and does not change account state.
_Avoid_: query

**Write tool**:
A tool that changes account state.
_Avoid_: mutation

## Conversation

**Message**:
The customer's latest input. One message produces one reply.
_Avoid_: reply, turn

**Attempt**:
One pass that may call tools for a message. A duplicate delivery is not a new attempt once a reply is stored. Taking over an expired claim is a new attempt.
_Avoid_: delivery, retry

**Reply**:
The customer-facing text published for one message.
_Avoid_: message

**Account fact**:
A statement about this customer's orders, refunds, balances, or profile. Only a successful tool result in this attempt can supply one.
_Avoid_: conversation text, a prior agent turn, or the customer's own wording as a source
