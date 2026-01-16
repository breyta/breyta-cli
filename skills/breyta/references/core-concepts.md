# Core concepts
- Flow definition: a versioned EDN map that describes triggers, steps, and orchestration.
- Flow profile: the prod instance with bindings and trigger state, pinned or auto-upgraded.
- Bindings: apply `:requires` slots and activation inputs (form values) for draft or prod.
- Activation: enables the prod profile after bindings are set; `--version` pins which version to run and does not accept inputs.
- Draft vs deployed: draft runs use draft bindings and draft version; deploy publishes an immutable version, activate enables prod.
- Triggers: manual, schedule, or event/webhook start a run.
- Versioning: deploy publishes a version; runs are pinned to the version that started them.
