# Define a skill's identity across copies

Type: grilling
Status: resolved
Blocked by: 06
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

When two different files declare the same skill name, are they separate skills, versions of one skill, or a conflict to resolve? Does "disable for all agents" target one shared source or every independent copy with that name? Which distinctions should the default list preserve?

## Answer

On October 10, 2026, the user chose **keep independent same-name skill copies separate, with a duplicate warning**.

Matching names do not establish identity. A global action on one selected copy must not silently include independent copies that happen to share its name. Shared references to one underlying source can be presented together; independent sources remain distinguishable until the user explicitly groups or reconciles them.
