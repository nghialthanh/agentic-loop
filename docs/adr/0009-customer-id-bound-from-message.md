# `customer_id` is bound from the message

Tools that take a customer id receive the id on the input message. The function schema sent to the model omits `customer_id`, and the executor inserts it before validation. A model-supplied customer id was rejected because the model could act on a customer who is not in this conversation.
