## What

<!-- One or two sentences: what changes and why. Link the issue if there is one. -->

## How to check

<!-- Commands or steps a reviewer can run. `make test` and, for engine changes, a `replay --decider momentum` before/after. -->

## Checklist

- [ ] `make vet test` passes
- [ ] No API key, database or personal data in the diff
- [ ] Guard rails still run after the decider (nothing bypasses `ApplyGuards` / `paper.Submit`)
- [ ] Docs updated if behaviour changed
