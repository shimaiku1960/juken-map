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

Claude Code owns the auto-memory directory. Do not update files there unless
the user explicitly asks to save or modify Claude's memory.
