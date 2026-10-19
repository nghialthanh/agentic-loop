# 04: Skill selection and general answer

**What to build:** The attempt calls the model for skill selection with candidate name and description only (no tool endpoints or parameters). Empty `skill_names` or undecodable selection JSON yields a general reply with no invented account facts, empty `skills` and `used_tools`, low confidence, and `error` null. Names not in the candidate list are dropped.

**Blocked by:** 03 (Skill catalog and top-20 candidates)

**Status:** ready-for-agent

- [ ] Selection prompt matches the foundation brief and sees only candidates
- [ ] Selection call has no tool schemas attached
- [ ] Empty skill list produces a general reply without account facts
- [ ] Invalid selection JSON is treated as no skill match
- [ ] Non-candidate skill names are dropped
- [ ] Selection is not retried against a larger catalog
- [ ] Tests use a mocked model through the attempt seam
