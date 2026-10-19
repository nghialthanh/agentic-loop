# 05: Read tool loop and grounded reply

**What to build:** After at least one skill is accepted, the attempt runs a bounded ReAct loop with that skill’s enabled tools. A status question can call `get_order_status`, normalize kept fields, and produce a reply whose account facts come only from successful tool results. Tools are attached only on the second model call. History is trimmed per the spec (newest 12 turns, then token budget).

**Blocked by:** 04 (Skill selection and general answer)

**Status:** ready-for-agent

- [ ] Tool-calling prompt is used only after skills are accepted
- [ ] Function schemas are built only from enabled tools of chosen skills
- [ ] Successful `get_order_status` grounds order account facts in the reply
- [ ] `used_tools` lists invocations in order with ok/error and latency
- [ ] Reply does not invent account facts absent from successful tool results
- [ ] History budget keeps newest turns and omits older turns for this call only
- [ ] At most four tool executions per attempt
- [ ] End-to-end test with mocked model and order API through the attempt seam
