# The executor does not interpret status

A successful required read is enough for the executor to allow the write. It does not refuse `refund_order` because the order is in transit or because a charge is unconfirmed. Eligibility stays with the order API. If that API rejects the refund, the HTTP call has happened and the tool is an error. A status policy inside the worker was rejected because it would copy rules the order API already owns.
