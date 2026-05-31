# Mnemon Goal Guide

This guide defines when project-scoped goal governance is useful.

## Stance

Use the goal loop when work spans multiple steps, needs durable evidence, or
should not be marked complete until explicit verification passes.

Prefer ordinary task execution for small one-shot work.

## Use Goal State

Use goal state when the current task needs one or more of:

- a durable objective outside the current host thread;
- a written plan that can survive context compaction or handoff;
- accepted evidence before completion;
- explicit verification and completion gates;
- a blocked, paused, or resumed state;
- a public link between Mnemon state and a host thread or goal id.

## Skip Goal State

Skip the goal loop when:

- the task is a direct one-step command;
- the user explicitly asks not to create durable state;
- the work is exploratory and has no completion gate;
- recording evidence would add noise without changing handoff or review.

## Host Boundary

Codex `/goal`, Claude Code, and other host continuation mechanisms remain
host-owned. Mnemon goal state is the durable project record. Do not write host
internal databases or private runtime state.

## Completion

A goal is not complete just because the host agent says the work is done. The
host agent must record evidence, run verification, and only then complete the
Mnemon goal.
