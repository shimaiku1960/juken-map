<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

## Project Memory

When resuming work, checking previous decisions, or reviewing the user's working
preferences, treat Claude Code's auto-memory as the source of truth:

`~/.claude/projects/-Users-ikuroshimamura-dev-product-juken-map/memory/MEMORY.md`

Read only the relevant topic files referenced from that file when additional
detail is needed. Do not use the repository-local `memory/MEMORY.md` as the
normal project memory; it records Claude-to-Codex migration state.

Claude Code owns the auto-memory directory. Claude Code and Codex may update
files there only when the user directly asks to save or modify project memory,
or when the automatic project-memory conditions below are satisfied.

Whenever Claude Code or Codex adds or changes shared auto-memory, visibly
attribute the changed content to the agent that wrote it and include an
absolute date. Use `**記入者: Claude Code（YYYY-MM-DD）**` or
`**記入者: Codex（YYYY-MM-DD）**` directly below a new heading. For an inline
change, append `（Claude Code更新: YYYY-MM-DD）` or
`（Codex更新: YYYY-MM-DD）`. Never relabel untouched content from the other
agent.

When an agent other than Claude Code or Codex (for example the Cursor agent)
records work on another agent's behalf — such as recovering a session that was
interrupted by a usage limit — attribute the memory to the agent that actually
did the work, and make clear that a different agent transcribed it. Use
`**記入者: <実作業エージェント>（<作業の性質>、<記録したエージェント>が履歴から代理記録: YYYY-MM-DD）**`
below a new heading, e.g.
`**記入者: Codex（ターミナル作業、Cursorが履歴から代理記録: 2026-07-11）**`.
For an inline change, append
`（<実作業エージェント>作業を<記録したエージェント>が代理記録: YYYY-MM-DD）`.

## Automatic Project Memory Updates

This section is standing explicit authorization for Claude Code and Codex to
update the shared project memory without asking for confirmation each time.

Before the final response, update project memory when a meaningful unit of work
has been completed, such as:

- implementing or fixing a feature;
- making a durable architecture or product decision;
- creating or merging a pull request; or
- completing a deployment or another operational milestone.

Do not update memory for answers to questions, read-only investigation, routine
test runs, intermediate progress, or unfinished implementation.

When an automatic update is required, follow these rules:

1. Treat the configured Claude Code auto-memory `MEMORY.md` as an index.
2. Update the most relevant existing topic file. Create a new topic file only
   when no suitable topic exists.
3. Record only durable context needed in a later session: completed work,
   decisions, verification results, remaining work, blockers, and warnings.
4. Remove or rewrite stale TODOs and statements that conflict with repository
   reality.
5. Keep the `MEMORY.md` index concise and update its topic summary to reflect
   the new current state.
6. Follow the author and absolute-date attribution rules above for every
   addition or modification. Attribute the change to the agent that actually
   made it.
7. If the configured external memory cannot be written, request the required
   permission. Never fall back to the repository-local `memory/MEMORY.md`.
8. In the final response, list the memory files that were updated.
