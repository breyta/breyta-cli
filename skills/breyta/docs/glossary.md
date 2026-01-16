# Glossary
- Flow definition: versioned EDN map that defines triggers, steps, and orchestration.
- Flow profile: prod instance with bindings and trigger state, pinned to a version.
- Bindings: applied slot values and activation inputs for a profile.
- Activation inputs: `:kind :form` values stored via bindings apply (not supplied at activate).
- Draft bindings: bindings used for draft runs only.
- Trigger routing: trigger store state updated on deploy.
