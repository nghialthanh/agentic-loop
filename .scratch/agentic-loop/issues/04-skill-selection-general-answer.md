# 04: Skill selection and general answer

**What to build:** The attempt calls the model for skill selection with candidate name and description only (no tool endpoints or parameters). Empty `skill_names` or undecodable selection JSON yields a general reply with no invented account facts, empty `skills` and `used_tools`, low confidence, and `error` null. Names not in the candidate list are dropped.

**Blocked by:** 03 (Skill catalog and top-20 candidates)

**Status:** resolved

- [x] Selection prompt matches the foundation brief and sees only candidates
- [x] Selection call has no tool schemas attached
- [x] Empty skill list produces a general reply without account facts
- [x] Invalid selection JSON is treated as no skill match
- [x] Non-candidate skill names are dropped
- [x] Selection is not retried against a larger catalog
- [x] Tests use a mocked model through the attempt seam

## Answer

Attempt seam now searches candidates, calls `Model.SelectSkills` once with the foundation system prompt and name/description-only candidates (`HasToolSchemas=false`), decodes JSON (invalid → no match), drops non-candidate names, and on empty skills returns `GeneralReply` with empty `skills`/`used_tools`, low confidence, and `error` null. Covered by mocked-model attempt tests.
