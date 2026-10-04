// Semantic Release configuration for go-decide.
//
// This file must stay at the repository root: semantic-release finds it with
// cosmiconfig, which searches the working directory and its parents. The
// extension is .cjs on purpose, because semantic-release loads the config with
// a require(), and a Go repository has no package.json to mark .js as ESM.
//
// This project publishes two things:
//
//   - the Go module, by pushing a vX.Y.Z git tag
//   - GitHub Releases carrying cross-compiled CLI binaries
//
// There is deliberately no @semantic-release/npm plugin: this is a Go module,
// not an npm package, so there is no package.json and nothing to publish to a
// registry. Release notes go in the GitHub Release body rather than a
// CHANGELOG file.
//
// Keep the commit subjects conventional so commit-analyzer can classify them:
//
//	feat: ...      minor
//	fix: ...       patch
//	feat!: ...     major, or add BREAKING CHANGE: to the body
//	docs: chore:   no release
module.exports = {
  branches: [
    "main",
    { name: "beta", prerelease: true },
    { name: "alpha", prerelease: true },
  ],
  plugins: [
    "@semantic-release/commit-analyzer",
    "@semantic-release/release-notes-generator",
    "@semantic-release/github",
  ],
};