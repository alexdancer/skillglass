# Skillglass

A personal tool for finding, reading, organizing, and controlling local agent skills, with potential for a public open-source release.

## Language

**Skill**:
A named bundle of instructions and supporting resources that an agent application can use to perform a particular kind of work.

**Agent application**:
A product such as Codex, Claude Code, or Hermes that discovers and uses skills. In this product, choosing an agent means choosing one of these applications unless a more specific scope is stated.
_Avoid_: Treating an application and one of its individual agent sessions as interchangeable.

**Project skill**:
A skill associated with a particular project rather than the user's machine-wide collection.

**Project override**:
A project-specific choice about a globally available skill, allowing its availability in that project to differ from the user's global choice.

**Independent copy**:
A separate source of skill content, even when it has the same name as another skill. Matching names alone do not make two copies interchangeable.

**Source**:
One skill bundle directory identified by its proven filesystem object when that evidence is available. A source can have several discovery references. Equal names, equal contents, or a shared manifest file do not make separate bundle directories one source.

**Discovery reference**:
One selected root's path to a candidate skill bundle, retaining its original location and discovery context even when it cannot resolve to a source. A resolved path is known only when resolution succeeds; reference provenance does not prove native ownership or availability.

**Shared reference**:
A second discovery reference proven to reach the same underlying skill bundle directory. Changes to that source can affect every application that uses a shared reference.
_Avoid_: Duplicate copy when no separate content exists.

**Disable**:
Prevent selected agent applications from using a skill while retaining its content. The requested scope may be one application or all applications.
_Avoid_: Delete, uninstall.

**Delete**:
Permanently remove a skill's content after confirmation, rather than only changing which applications can use it. There is no built-in recovery for this action.
_Avoid_: Disable.
