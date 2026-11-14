package punchflow

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRepoInvariants(t *testing.T) {
	t.Run("cf01_ban", func(t *testing.T) {
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`ticket_submitted|ticket_remanded|ticket_resubmitted|comment_deleted|ticket_admin|audit_read`),
			regexp.MustCompile(`\b(user|org|workflow|role|file|proxy)_(read|write)\b`),
			regexp.MustCompile(`StepTypeGrade|StepTypeParallel|StepStatus(Approved|Denied|Remanded)|TicketStatusWithdrawn`),
			regexp.MustCompile(`"userType": *"member"|"status": *"active"|"sectionType": *"conditional"|"gradeSymbol": *""`),
		}
		for _, root := range []string{"internal", "fixtures"} {
			pf27Walk(t, root, func(path string, entry fs.DirEntry) {
				if !entry.Type().IsRegular() || strings.HasSuffix(path, "_test.go") || strings.HasPrefix(path, "internal/fixtures/testdata/invalid/") {
					return
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("read %s: %v", path, err)
					return
				}
				for lineNumber, line := range strings.Split(string(contents), "\n") {
					for _, pattern := range patterns {
						if pattern.MatchString(line) {
							t.Errorf("%s:%d: %s", path, lineNumber+1, line)
							break
						}
					}
				}
			})
		}
	})

	t.Run("negative_tests_present", func(t *testing.T) {
		for _, path := range []string{"internal/fixtures/enum_rules_test.go", "internal/api/v1/roles_test.go"} {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("read %s: %v", path, err)
				continue
			}
			if !strings.Contains(string(contents), "audit_read") {
				t.Errorf("%s does not contain audit_read", path)
			}
		}
		found := false
		pf27Walk(t, "internal/admin", func(path string, entry fs.DirEntry) {
			if !entry.Type().IsRegular() || !strings.HasSuffix(path, "_test.go") {
				return
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("read %s: %v", path, err)
				return
			}
			if strings.Contains(string(contents), "ticket_submitted") || strings.Contains(string(contents), "comment_deleted") {
				found = true
			}
		})
		if !found {
			t.Error("no internal/admin test contains ticket_submitted or comment_deleted")
		}
	})

	t.Run("go_file_size", func(t *testing.T) {
		pf27Walk(t, ".", func(path string, entry fs.DirEntry) {
			if !entry.Type().IsRegular() || filepath.Ext(path) != ".go" {
				return
			}
			file, err := os.Open(path)
			if err != nil {
				t.Errorf("open %s: %v", path, err)
				return
			}
			scanner := bufio.NewScanner(file)
			lineCount := 0
			for scanner.Scan() {
				lineCount++
			}
			scanErr := scanner.Err()
			closeErr := file.Close()
			if scanErr != nil {
				t.Errorf("scan %s: %v", path, scanErr)
				return
			}
			if closeErr != nil {
				t.Errorf("close %s: %v", path, closeErr)
				return
			}
			if lineCount >= 1000 {
				t.Errorf("%s has %d lines; Go files must have fewer than 1000", path, lineCount)
			}
		})
	})

	t.Run("no_foreign_lockfiles", func(t *testing.T) {
		for _, path := range []string{"web/package-lock.json", "web/yarn.lock", "web/pnpm-lock.yaml"} {
			_, err := os.Stat(path)
			if err == nil {
				t.Errorf("foreign lockfile exists: %s", path)
			} else if !os.IsNotExist(err) {
				t.Errorf("stat %s: %v", path, err)
			}
		}
	})
}

func pf27Walk(t *testing.T, root string, visit func(string, fs.DirEntry)) {
	t.Helper()
	err := fs.WalkDir(os.DirFS("."), root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			t.Errorf("walk %s: %v", path, walkErr)
			return nil
		}
		if entry.IsDir() {
			cleanPath := filepath.ToSlash(path)
			base := filepath.Base(cleanPath)
			if base == ".git" || base == "tmp" || cleanPath == "web/node_modules" || cleanPath == "web/dist" {
				return fs.SkipDir
			}
		}
		visit(filepath.ToSlash(path), entry)
		return nil
	})
	if err != nil {
		t.Errorf("walk %s: %v", root, err)
	}
}
