# Define the purpose, audience, and control scope

Type: grilling
Status: resolved
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

What should the tool help the user accomplish, who is it for, which controls belong in the product direction, and what interface should it offer?

## Answer

The user's October 10, 2026 answers establish:

- Finding and reading installed skills, cleaning up skills, and controlling which agents use them are all core goals.
- Build for the user's own setup first, with potential for a fully open-source tool later.
- Allow selecting the agents that may use a skill, disabling a skill for a specific agent, disabling it for all agents, deleting an entire skill, and working with project-specific skills.
- Offer both an interactive terminal browser and ordinary CLI commands.
- Go is the preferred implementation language, as stated in the original request.

These answers do not settle deletion recovery, project override behavior, same-name skill identity, the supported agent list, plugin handling, privacy, or first-release feature priority.

## Comments

Recorded from the user's answers in this conversation. No implementation is authorized by this resolution; planning remains active.
