# SDD ledger — plan: docs/superpowers/plans/2026-07-27-startup-phase-unification.md

Worktree: /Users/marten/GolandProjects/noda/.worktrees/startup-phase-unification
Branch: feat/startup-phase-unification
Issue: #456

Pre-flight scan: two plan defects found and fixed before Task 1 (commit below).
  - Task 6 had Step 4 write a runSchedules sketch that Step 5 immediately
    rewrote, including a dead `_ = i` line. Reordered: typed errors first,
    then the phases written once, correctly.
  - Task 7 Step 5 deleted internal/validate while internal/mcp still imported
    it, leaving the tree unbuildable between two tasks. The deletion moved to
    Task 8, after its last caller is migrated.
