# Releasing Process

- Add release entry to [changelog](./CHANGELOG.md)
- Open a PR with the above, and merge that into main
- Create new tag on merged commit with the new version (e.g. `v0.2.1`)
- Push the tag upstream (this will kick off the release pipeline in CI)
- Copy change log entry for newest version into draft GitHub release created as part of CI publish steps
- Update [public docs](https://github.com/honeycombio/docs/blob/main/scripts)
