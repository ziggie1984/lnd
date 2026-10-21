package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// lines returns its arguments as the lines of a test document.
func lines(l ...string) []string {
	return l
}

// TestFoldFragment checks that fragment entries are appended to the section
// with the same heading path and that the rest of the document is unchanged.
func TestFoldFragment(t *testing.T) {
	t.Parallel()

	doc := lines(
		"# Release Notes",
		"",
		"# Bug Fixes",
		"",
		"* existing fix",
		"",
		"",
		"# Improvements",
		"",
		"## Code Health",
		"",
		"## Deprecations",
		"",
		"### Warning",
		"",
		"# Technical and Architectural Updates",
		"",
		"## Code Health",
		"",
		"```text",
		"# not a heading",
		"```",
		"",
		"# Contributors (Alphabetical Order)",
	)

	tests := []struct {
		name     string
		fragment []string
		want     []string
		err      string
	}{{
		name: "append to non-empty section",
		fragment: lines(
			"# Bug Fixes", "", "* new fix", "  continued", "",
		),
		want: lines(
			"# Release Notes", "",
			"# Bug Fixes", "", "* existing fix", "",
			"* new fix", "  continued", "",
			"# Improvements", "", "## Code Health", "",
			"## Deprecations", "", "### Warning", "",
			"# Technical and Architectural Updates", "",
			"## Code Health", "",
			"```text", "# not a heading", "```", "",
			"# Contributors (Alphabetical Order)",
		),
	}, {
		name: "same heading under different parents",
		fragment: lines(
			"# Technical and Architectural Updates", "",
			"## Code Health", "", "* technical",
			"# Improvements", "", "## Code Health", "",
			"* improvement",
		),
		want: lines(
			"# Release Notes", "",
			"# Bug Fixes", "", "* existing fix", "", "",
			"# Improvements", "", "## Code Health", "",
			"* improvement", "",
			"## Deprecations", "", "### Warning", "",
			"# Technical and Architectural Updates", "",
			"## Code Health", "",
			"```text", "# not a heading", "```", "",
			"* technical", "",
			"# Contributors (Alphabetical Order)",
		),
	}, {
		name: "entries go above nested headings",
		fragment: lines(
			"# Improvements", "", "## Deprecations", "",
			"* deprecated",
		),
		want: lines(
			"# Release Notes", "",
			"# Bug Fixes", "", "* existing fix", "", "",
			"# Improvements", "", "## Code Health", "",
			"## Deprecations", "", "* deprecated", "",
			"### Warning", "",
			"# Technical and Architectural Updates", "",
			"## Code Health", "",
			"```text", "# not a heading", "```", "",
			"# Contributors (Alphabetical Order)",
		),
	}, {
		name: "headings in fragment code blocks are entries",
		fragment: lines(
			"# Bug Fixes", "", "```text", "# Missing", "```",
		),
		want: lines(
			"# Release Notes", "",
			"# Bug Fixes", "", "* existing fix", "",
			"```text", "# Missing", "```", "",
			"# Improvements", "", "## Code Health", "",
			"## Deprecations", "", "### Warning", "",
			"# Technical and Architectural Updates", "",
			"## Code Health", "",
			"```text", "# not a heading", "```", "",
			"# Contributors (Alphabetical Order)",
		),
	}, {
		name: "missing section",
		fragment: lines(
			"# Bug Fixes", "", "## Unknown", "", "* entry",
		),
		err: `section "# Bug Fixes > ## Unknown" not found`,
	}, {
		name:     "text before the first heading",
		fragment: lines("* entry", "", "# Bug Fixes"),
		err:      "text before the first heading",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := foldFragment(doc, test.fragment)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

// TestFoldFragmentsInOrder checks that entries of several fragments in the
// same section keep the order in which the fragments are folded.
func TestFoldFragmentsInOrder(t *testing.T) {
	t.Parallel()

	doc := lines("# Bug Fixes", "", "# New Features")
	for _, entry := range []string{"* first", "* second"} {
		var err error
		doc, err = foldFragment(doc, lines("# Bug Fixes", "", entry))
		require.NoError(t, err)
	}

	require.Equal(t, lines(
		"# Bug Fixes", "", "* first", "", "* second", "",
		"# New Features",
	), doc)
}

// TestContributors checks that the contributor list merges the listed names
// with the Git authors.
func TestContributors(t *testing.T) {
	t.Parallel()

	doc := lines(
		"# Bug Fixes", "",
		"# Contributors (Alphabetical Order)", "",
		"* Ziggie", "* alice",
	)
	authors := lines(
		"ziggie", "Bob", "dependabot[bot]", "Alice", "", "Carol",
	)

	names := mergeNames(contributors(doc), authors)
	require.Equal(t, lines("alice", "Bob", "Carol", "Ziggie"), names)

	require.Equal(t, lines(
		"# Bug Fixes", "",
		"# Contributors (Alphabetical Order)", "",
		"* alice", "* Bob", "* Carol", "* Ziggie",
	), setContributors(doc, names))
}

// TestSetContributors checks where the contributor list is written.
func TestSetContributors(t *testing.T) {
	t.Parallel()

	// A section after the contributors stays in place.
	doc := lines(
		"# Contributors (Alphabetical Order)", "", "* Old", "",
		"# Appendix",
	)
	require.Equal(t, lines(
		"# Contributors (Alphabetical Order)", "", "* New", "",
		"# Appendix",
	), setContributors(doc, lines("New")))

	// A document without contributors section gets one appended.
	doc = lines("# Bug Fixes", "", "* fix", "")
	require.Equal(t, lines(
		"# Bug Fixes", "", "* fix", "",
		"# Contributors (Alphabetical Order)", "", "* New",
	), setContributors(doc, lines("New")))
}

// TestLinkedPRs checks that PR links are matched by their exact number.
func TestLinkedPRs(t *testing.T) {
	t.Parallel()

	content := strings.Join(lines(
		"[a](https://github.com/lightningnetwork/lnd/pull/11327)",
		"[b](https://github.com/lightningnetwork/lnd/pull/113270)",
		"[c](https://github.com/lightning/bolts/pull/1059)",
	), "\n")

	require.Equal(t, map[int]bool{11327: true, 113270: true},
		linkedPRs(content))
}
