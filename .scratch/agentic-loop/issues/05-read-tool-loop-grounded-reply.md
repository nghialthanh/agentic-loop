# 05: Read tool loop and grounded reply

**What to build:** After at least one skill is accepted, the attempt runs a bounded ReAct loop with that skill’s enabled tools. A status question can call `get_order_status`, normalize kept fields, and produce a reply whose account facts come only from successful tool results. Tools are attached only on the second model call. History is trimmed per the spec (newest 12 turns, then token budget).

**Blocked by:** 04 (Skill selection and general answer)

**Status:** resolved

- [x] Tool-calling prompt is used only after skills are accepted
- [x] Function schemas are built only from enabled tools of chosen skills
- [x] Successful `get_order_status` grounds order account facts in the reply
- [x] `used_tools` lists invocations in order with ok/error and latency
- [x] Reply does not invent account facts absent from successful tool results
- [x] History budget keeps newest turns and omits older turns for this call only
- [x] At most four tool executions per attempt
- [x] End-to-end test with mocked model and order API through the attempt seam

## Answer

After accepted skills, the attempt loads enabled tools, builds function schemas, trims history (newest 12 then token budget with an omission line for this call only), and runs a bounded ReAct loop via `Model.CallWithTools` + `Tools.Execute` (max four HTTP executions). Successful `get_order_status` keep fields ground the reply; invented account facts are replaced from tool keeps. Covered by attempt-seam tests with a scripted model and recording order API.
