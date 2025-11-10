# Release Process

### 1. Update version number in code base and dist/

To release a new version, change version at the following files:
- main.go
- dist/get.sh
- dist/index.html
- version.txt

The automated way is:

```bash
./scripts/release.sh <version number> # ie. v0.02.00
```

When in doubt, look at the script's help output:

```bash
./scripts/release.sh help
```

### 2. Update CHANGELOG.md

```
## [v0.01.00] - <DATE ~ 2025-11-06>



[v0.01.00]: https://github.com/hkdb/otui/releases/tag/v0.01.00
```

### 3. Create tag based on version.


### 4. Create release based on tag


### 5. Monitor Github Actions job to make sure all is well.
