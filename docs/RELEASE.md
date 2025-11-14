# Release Process

### 1. Update CHANGELOG.md

```
## [v0.01.00] - <DATE ~ 2025-11-06>



[v0.01.00]: https://github.com/hkdb/otui/releases/tag/v0.01.00
```

### 2. Update version number in code base and dist/

To release a new version, change version at the following files:
- main.go
- dist/get.sh
- dist/index.html
- version.txt

The automated way is:

```bash
./scripts/release.sh <version number> # ex. v0.02.00
```

When in doubt, look at the script's help output:

```bash
./scripts/release.sh help
```

### 3. Build and push Docker image

```bash
go build
./scripts/build-container.sh <version number> # ex. v0.02.00
```
Test image before push.
Test image on test devices before tagging as latest and push.

### 4. Create tag and release based on version.


### 5. Monitor Github Actions job to make sure all is well.
