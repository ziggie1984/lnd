package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	notesDir            = "docs/release-notes"
	unreleasedDir       = notesDir + "/unreleased"
	templatePath        = notesDir + "/release-notes-template.md"
	contributorsHeading = "# Contributors (Alphabetical Order)"
)

var (
	versionPattern  = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	fragmentPattern = regexp.MustCompile(`^(\d+)\.md$`)
	headingPattern  = regexp.MustCompile(`^#+ `)
	prLinkPattern   = regexp.MustCompile(`lightningnetwork/lnd/pull/(\d+)`)
)

// main folds the release note fragments in docs/release-notes/unreleased into
// the release note document of a version, as described in
// docs/release-notes/README.md. It is run from the repository root:
//
//	go run ./scripts/release-notes <version> [<from>]
//
// The entries of each fragment are appended to the sections of
// release-notes-<version>.md with the same heading path, which is created
// from the template if it doesn't exist yet. Fragments whose pull request is
// already announced in another release note document are deleted instead.
// The contributor list is regenerated from the Git authors of <from>..HEAD,
// where <from> defaults to the latest final release tag. Nothing is written
// if a fragment can't be folded.
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: release-notes <version> [<from>]")
	}

	version := args[0]
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("version must be of the form "+
			"<major>.<minor>.<patch>, e.g. 0.21.5, got %q", version)
	}

	var from string
	if len(args) == 2 {
		from = args[1]
	} else {
		tag, err := git("describe", "--tags", "--abbrev=0",
			"--match", "v*-beta")
		if err != nil {
			return fmt.Errorf("no release tag found, specify "+
				"<from> explicitly: %w", err)
		}
		from = strings.TrimSpace(tag)
	}

	authors, err := git("log", "--no-merges", "--format=%aN",
		from+"..HEAD")
	if err != nil {
		return err
	}

	docPath := filepath.Join(notesDir, "release-notes-"+version+".md")
	content, err := os.ReadFile(docPath)
	if errors.Is(err, os.ErrNotExist) {
		logf("Creating %s from %s", docPath, templatePath)
		content, err = os.ReadFile(templatePath)
	}
	if err != nil {
		return err
	}
	doc := splitLines(string(content))

	released, err := releasedPRs(docPath)
	if err != nil {
		return err
	}

	fragments, err := listFragments()
	if err != nil {
		return err
	}

	for _, f := range fragments {
		// A change on master that was backported to a patch release of
		// the previous version must not be announced again.
		if released[f.pr] {
			logf("Deleting %s, PR %d is already announced in "+
				"an earlier release", f.path, f.pr)
			continue
		}

		// The baseline of a release that was already being written when
		// fragments were introduced may contain the same entry.
		if linkedPRs(strings.Join(doc, "\n"))[f.pr] {
			logf("Warning: %s already mentions PR %d, check "+
				"for a duplicate entry", docPath, f.pr)
		}

		fragment, err := os.ReadFile(f.path)
		if err != nil {
			return err
		}

		doc, err = foldFragment(doc, splitLines(string(fragment)))
		if err != nil {
			return fmt.Errorf("%s: %w, add the section to %s",
				f.path, err, docPath)
		}
		logf("Folding %s", f.path)
	}

	names := mergeNames(contributors(doc), splitLines(authors))
	doc = setContributors(doc, names)
	logf("Updated contributors from %s..HEAD", from)

	// Only write once all fragments were folded, so a failure leaves the
	// release notes untouched.
	err = os.WriteFile(docPath, []byte(strings.Join(doc, "\n")+"\n"), 0644)
	if err != nil {
		return err
	}
	for _, f := range fragments {
		if err := os.Remove(f.path); err != nil {
			return err
		}
	}

	logf("Review %s: edit ordering and transitions as needed, and "+
		"check co-authored commits and the contributor list", docPath)

	return nil
}

// logf prints a progress message.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// git runs a git command and returns its output.
func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "),
			err)
	}

	return string(out), nil
}

// fragment is a release note fragment named after its pull request.
type fragment struct {
	path string
	pr   int
}

// listFragments returns the fragments ordered by pull request number.
func listFragments() ([]fragment, error) {
	paths, err := filepath.Glob(filepath.Join(unreleasedDir, "*.md"))
	if err != nil {
		return nil, err
	}

	var fragments []fragment
	for _, path := range paths {
		match := fragmentPattern.FindStringSubmatch(filepath.Base(path))
		if match == nil {
			return nil, fmt.Errorf("%s: fragment must be named "+
				"<PR number>.md", path)
		}

		pr, _ := strconv.Atoi(match[1])
		fragments = append(fragments, fragment{path: path, pr: pr})
	}

	sort.Slice(fragments, func(i, j int) bool {
		return fragments[i].pr < fragments[j].pr
	})

	return fragments, nil
}

// releasedPRs returns the pull requests announced in the release note
// documents other than the given one.
func releasedPRs(docPath string) (map[int]bool, error) {
	docs, err := filepath.Glob(filepath.Join(notesDir,
		"release-notes-[0-9]*.md"))
	if err != nil {
		return nil, err
	}

	released := make(map[int]bool)
	for _, path := range docs {
		if path == docPath {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for pr := range linkedPRs(string(content)) {
			released[pr] = true
		}
	}

	return released, nil
}

// linkedPRs returns the pull requests the content links to.
func linkedPRs(content string) map[int]bool {
	prs := make(map[int]bool)
	for _, match := range prLinkPattern.FindAllStringSubmatch(content, -1) {
		pr, _ := strconv.Atoi(match[1])
		prs[pr] = true
	}

	return prs
}

// section is a heading and the lines up to the next heading. Its path is
// "# Section" for top-level headings and "# Section > ## Subsection" for
// nested ones, matching check-release-notes.sh. The lines before the first
// heading form a section with an empty path.
type section struct {
	path  string
	start int
	lines []string
}

// sections splits a document into sections. Headings inside fenced code
// blocks don't start a new section.
func sections(lines []string) []section {
	all := []section{{}}
	top, fenced := "", false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}

		if !fenced && headingPattern.MatchString(line) {
			path := line
			if strings.HasPrefix(line, "# ") {
				top = line
			} else {
				path = top + " > " + line
			}
			all = append(all, section{path: path, start: i})
		}

		last := &all[len(all)-1]
		last.lines = append(last.lines, line)
	}

	return all
}

// foldFragment appends the entries of each section of the fragment to the
// section of the document with the same heading path.
func foldFragment(doc, fragment []string) ([]string, error) {
	for i, s := range sections(fragment) {
		// The fragment must start with a heading.
		if i == 0 {
			if len(trimBlank(s.lines)) != 0 {
				return nil, errors.New("text before the " +
					"first heading")
			}

			continue
		}

		entries := trimBlank(s.lines[1:])
		if len(entries) == 0 {
			continue
		}

		var err error
		doc, err = appendToSection(doc, s.path, entries)
		if err != nil {
			return nil, err
		}
	}

	return doc, nil
}

// appendToSection appends the entries to the end of the first section with
// the given path, separated by blank lines.
func appendToSection(doc []string, path string, entries []string) ([]string,
	error) {

	all := sections(doc)
	for i, s := range all {
		if s.path == "" || s.path != path {
			continue
		}

		end := s.start + len(trimBlank(s.lines))
		rest := doc[s.start+len(s.lines):]

		result := append([]string{}, doc[:end]...)
		result = append(result, "")
		result = append(result, entries...)
		if i < len(all)-1 {
			result = append(result, "")
		}

		return append(result, rest...), nil
	}

	return nil, fmt.Errorf("section %q not found", path)
}

// contributors returns the names listed in the contributors section.
func contributors(doc []string) []string {
	var names []string
	for _, s := range sections(doc) {
		if !strings.HasPrefix(s.path, "# Contributors") {
			continue
		}

		for _, line := range s.lines[1:] {
			name, ok := strings.CutPrefix(line, "* ")
			if !ok {
				name, ok = strings.CutPrefix(line, "- ")
			}
			if ok {
				names = append(names, name)
			}
		}
	}

	return names
}

// mergeNames merges the listed names with the Git authors and sorts them
// case-insensitively. Names that only differ in case are listed once,
// keeping the listed spelling, and bots are skipped. Several names of one
// person are mapped to a single one in .mailmap.
func mergeNames(listed, authors []string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, name := range append(listed, authors...) {
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if name == "" || strings.Contains(name, "[bot]") || seen[key] {
			continue
		}

		seen[key] = true
		names = append(names, name)
	}

	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	return names
}

// setContributors replaces the contributors section with the given names,
// appending the section if the document doesn't have one.
func setContributors(doc, names []string) []string {
	list := []string{contributorsHeading, ""}
	for _, name := range names {
		list = append(list, "* "+name)
	}

	all := sections(doc)
	for i, s := range all {
		if !strings.HasPrefix(s.path, "# Contributors") {
			continue
		}

		result := append([]string{}, doc[:s.start]...)
		result = append(result, list...)
		if i < len(all)-1 {
			result = append(result, "")
		}

		return append(result, doc[s.start+len(s.lines):]...)
	}

	return append(append(trimBlank(doc), ""), list...)
}

// splitLines splits text into lines without line terminators.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}

	return strings.Split(text, "\n")
}

// trimBlank removes leading and trailing blank lines.
func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}
