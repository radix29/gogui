// Package repocheck holds the repository consistency tests (review R14):
// docs/parity.md and its pointers, THIRD_PARTY.md against go.mod, the
// examples listed where they are registered, the file tables of PLAN.md and
// ARCHITECTURE.md against the tree, and the architecture overview that the
// two files share. They read files only, so they live outside the root
// package and its stress run. The package has no code but its tests; its
// TestMain runs them from the repository root.
package repocheck
