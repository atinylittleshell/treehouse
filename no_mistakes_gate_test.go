package main_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// requiredCheckContext is the status check context this repository's main
// ruleset requires. The workflow job name must match it exactly, or the
// ruleset waits forever on a check nothing ever reports.
const requiredCheckContext = "PR must be raised via no-mistakes"

const gateWorkflowPath = ".github/workflows/no-mistakes-required.yml"

// gateStep is the one workflow step that decides the required check.
type gateStep struct {
	env map[string]string
	run string
}

type workflowFile struct {
	Jobs map[string]struct {
		Name  string `yaml:"name"`
		If    string `yaml:"if"`
		Steps []struct {
			Name string            `yaml:"name"`
			Run  string            `yaml:"run"`
			Env  map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// loadGateStep reads the real workflow and returns the step that runs the gate,
// so the test drives the shipped configuration rather than a copy of it.
func loadGateStep(t *testing.T) gateStep {
	t.Helper()

	data, err := os.ReadFile(gateWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", gateWorkflowPath, err)
	}
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parse %s: %v", gateWorkflowPath, err)
	}

	for jobID, job := range wf.Jobs {
		if job.Name != requiredCheckContext {
			continue
		}
		// Every exemption must live in the script, not in a job-level `if:`,
		// so the gate has a single executable decision surface.
		if strings.TrimSpace(job.If) != "" {
			t.Fatalf("job %q must not carry a job-level if:, it decides exemptions in the gate script", jobID)
		}
		for _, step := range job.Steps {
			if strings.TrimSpace(step.Run) == "" {
				continue
			}
			return gateStep{env: step.Env, run: step.Run}
		}
		t.Fatalf("job %q has no run: step", jobID)
	}

	t.Fatalf("%s has no job named %q", gateWorkflowPath, requiredCheckContext)
	return gateStep{}
}

var expressionPattern = regexp.MustCompile(`\$\{\{\s*([^}]+?)\s*\}\}`)

// pullRequestEvent is the subset of a GitHub `pull_request` event payload the
// gate step reads through its env: block.
type pullRequestEvent struct {
	number   int
	body     string
	author   string
	headRef  string
	headRepo string
	headSHA  string
	baseRepo string
}

func (e pullRequestEvent) lookup(path string) (string, bool) {
	switch path {
	case "github.event.pull_request.number":
		return fmt.Sprint(e.number), true
	case "github.event.pull_request.body":
		return e.body, true
	case "github.event.pull_request.user.login":
		return e.author, true
	case "github.event.pull_request.head.ref":
		return e.headRef, true
	case "github.event.pull_request.head.repo.full_name":
		return e.headRepo, true
	case "github.event.pull_request.head.sha":
		return e.headSHA, true
	case "github.event.pull_request.base.repo.full_name":
		return e.baseRepo, true
	default:
		return "", false
	}
}

// resolveEnv expands the step's `${{ ... }}` expressions against the event, the
// same substitution the Actions runner performs before running the step.
func resolveEnv(t *testing.T, step gateStep, event pullRequestEvent) []string {
	t.Helper()

	if len(step.env) == 0 {
		t.Fatal("gate step declares no env:, so it can never see the pull request")
	}

	out := make([]string, 0, len(step.env))
	for name, raw := range step.env {
		value := expressionPattern.ReplaceAllStringFunc(raw, func(match string) string {
			path := expressionPattern.FindStringSubmatch(match)[1]
			resolved, ok := event.lookup(path)
			if !ok {
				t.Fatalf("env %s references unsupported expression %q", name, path)
			}
			return resolved
		})
		out = append(out, name+"="+value)
	}
	return out
}

// runGate executes the workflow step's shell exactly as the runner would, from
// the repository root, and reports whether the required check would pass.
func runGate(t *testing.T, step gateStep, event pullRequestEvent) (bool, string) {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available; the gate step runs on ubuntu-latest")
	}

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	cmd := exec.Command(bash, "-e", "-c", step.run)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), resolveEnv(t, step, event)...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, string(output)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, string(output)
	}
	t.Fatalf("run gate step: %v (%s)", err, output)
	return false, ""
}

const (
	noMistakesSignature = "## Pipeline\n\nUpdates from [git push no-mistakes](https://github.com/kunchenguid/no-mistakes)\n"
	// attestationHeadSHA is the compact v1 comment's head_sha. Passing cases
	// bind github.event.pull_request.head.sha to this value.
	attestationHeadSHA = "0123456789abcdef0123456789abcdef01234567"
	otherHeadSHA       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// completedAttestation matches the compact v1 comment no-mistakes >= 1.46.0 writes.
	// pr/ci are commonly still running/pending at PR write time and are not required.
	completedAttestation = `<!-- no-mistakes-pipeline-attestation:v1 {"head_sha":"0123456789abcdef0123456789abcdef01234567","steps":[{"step":"intent","status":"completed"},{"step":"rebase","status":"completed"},{"step":"review","status":"completed"},{"step":"test","status":"completed"},{"step":"document","status":"completed"},{"step":"lint","status":"completed"},{"step":"push","status":"completed"},{"step":"pr","status":"running"},{"step":"ci","status":"pending"}]} -->`
	noMistakesBody       = noMistakesSignature + "\n" + completedAttestation + "\n"
	releaseBody          = ":robot: I have created a release *beep* *boop*\n---\n\n## [2.1.1](https://example.invalid)\n\n---\n" +
		"This PR was generated with [Release Please](https://github.com/googleapis/release-please). See [documentation](https://github.com/googleapis/release-please#release-please)."
	repo     = "kunchenguid/treehouse"
	forkRepo = "someone-else/treehouse"

	attestationVersionFloor = "no-mistakes >= 1.46.0"
	attestationVersionURL   = "https://github.com/kunchenguid/no-mistakes/pull/670"
)

func TestNoMistakesGateDecisions(t *testing.T) {
	step := loadGateStep(t)

	cases := []struct {
		name       string
		event      pullRequestEvent
		pass       bool
		wantOutput []string
		hideOutput []string
	}{
		{
			// The live shape of treehouse's own release PRs (see PR #78).
			name: "release-please release PR is exempt",
			event: pullRequestEvent{
				number: 78, body: releaseBody, author: "github-actions[bot]",
				headRef: "release-please--branches--main", headRepo: repo, baseRepo: repo,
			},
			pass: true,
		},
		{
			// Structural, not identity: the same PR under a PAT identity, which
			// is what release-please would become if it ever used one.
			name: "release-please release PR authored by a human identity is exempt",
			event: pullRequestEvent{
				number: 79, body: releaseBody, author: "kunchenguid",
				headRef: "release-please--branches--main", headRepo: repo, baseRepo: repo,
			},
			pass: true,
		},
		{
			name: "legacy release-please branch prefix is exempt",
			event: pullRequestEvent{
				number: 80, body: releaseBody, author: "kunchenguid",
				headRef: "release-please/branches/main", headRepo: repo, baseRepo: repo,
			},
			pass: true,
		},
		{
			name: "github-actions bot is exempt",
			event: pullRequestEvent{
				number: 81, body: "chore: routine bot update", author: "github-actions[bot]",
				headRef: "chore/bot", headRepo: repo, baseRepo: repo,
			},
			pass: true,
		},
		{
			name: "dependabot is exempt",
			event: pullRequestEvent{
				number: 82, body: "Bumps a dependency.", author: "dependabot[bot]",
				headRef: "dependabot/go_modules/example-1.2.3", headRepo: repo, baseRepo: repo,
			},
			pass: true,
		},
		{
			name: "github-actions bot remains exempt even with a signature-only body",
			event: pullRequestEvent{
				number: 96, body: noMistakesSignature, author: "github-actions[bot]",
				headRef: "chore/bot-with-signature", headRepo: repo, baseRepo: repo,
			},
			pass: true,
		},
		{
			name: "human PR carrying the no-mistakes signature and completed attestation passes",
			event: pullRequestEvent{
				number: 83, body: noMistakesBody, author: "kunchenguid",
				headRef: "fm/some-work", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass: true,
		},
		{
			name: "human PR with the signature but no attestation fails",
			event: pullRequestEvent{
				number: 89, body: noMistakesSignature, author: "kunchenguid",
				headRef: "fm/old-no-mistakes", headRepo: repo, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{attestationVersionFloor, attestationVersionURL, "Older no-mistakes that only writes the signature line is not enough"},
		},
		{
			name: "human PR with unparseable attestation JSON fails",
			event: pullRequestEvent{
				number:  90,
				body:    noMistakesSignature + "\n<!-- no-mistakes-pipeline-attestation:v1 {not-json} -->\n",
				author:  "kunchenguid",
				headRef: "fm/bad-attestation", headRepo: repo, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{attestationVersionFloor, attestationVersionURL},
		},
		{
			name: "human PR with skipped review fails",
			event: pullRequestEvent{
				number:  91,
				body:    noMistakesSignature + "\n" + strings.Replace(completedAttestation, `"step":"review","status":"completed"`, `"step":"review","status":"skipped"`, 1) + "\n",
				author:  "kunchenguid",
				headRef: "fm/skipped-review", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{"review: skipped", "Quota skips and agent skips are not compliant"},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR with failed test fails",
			event: pullRequestEvent{
				number:  92,
				body:    noMistakesSignature + "\n" + strings.Replace(completedAttestation, `"step":"test","status":"completed"`, `"step":"test","status":"failed"`, 1) + "\n",
				author:  "kunchenguid",
				headRef: "fm/failed-test", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{"test: failed"},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR with pending document fails",
			event: pullRequestEvent{
				number:  93,
				body:    noMistakesSignature + "\n" + strings.Replace(completedAttestation, `"step":"document","status":"completed"`, `"step":"document","status":"pending"`, 1) + "\n",
				author:  "kunchenguid",
				headRef: "fm/pending-document", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{"document: pending"},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR with running document fails",
			event: pullRequestEvent{
				number:  94,
				body:    noMistakesSignature + "\n" + strings.Replace(completedAttestation, `"step":"document","status":"completed"`, `"step":"document","status":"running"`, 1) + "\n",
				author:  "kunchenguid",
				headRef: "fm/running-document", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{"document: running"},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR missing the document step fails",
			event: pullRequestEvent{
				number:  95,
				body:    noMistakesSignature + "\n<!-- no-mistakes-pipeline-attestation:v1 {\"head_sha\":\"0123456789abcdef0123456789abcdef01234567\",\"steps\":[{\"step\":\"review\",\"status\":\"completed\"},{\"step\":\"test\",\"status\":\"completed\"}]} -->\n",
				author:  "kunchenguid",
				headRef: "fm/missing-document", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass:       false,
			wantOutput: []string{"document: missing"},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR with attestation for a different head SHA fails",
			event: pullRequestEvent{
				number: 97, body: noMistakesBody, author: "kunchenguid",
				headRef: "fm/stale-attestation", headRepo: repo, headSHA: otherHeadSHA, baseRepo: repo,
			},
			pass: false,
			wantOutput: []string{
				"not bound to the current pull request head",
				"stale attestation",
				attestationHeadSHA,
				otherHeadSHA,
			},
			hideOutput: []string{attestationVersionFloor, "review: skipped"},
		},
		{
			name: "human PR with empty attestation head_sha fails",
			event: pullRequestEvent{
				number:  98,
				body:    noMistakesSignature + "\n" + strings.Replace(completedAttestation, `"head_sha":"0123456789abcdef0123456789abcdef01234567"`, `"head_sha":""`, 1) + "\n",
				author:  "kunchenguid",
				headRef: "fm/empty-head-sha", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass: false,
			wantOutput: []string{
				"not bound to the current pull request head",
				"attestation head_sha: (missing)",
				attestationHeadSHA,
			},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR with attestation JSON missing head_sha fails",
			event: pullRequestEvent{
				number:  99,
				body:    noMistakesSignature + "\n" + strings.Replace(completedAttestation, `"head_sha":"0123456789abcdef0123456789abcdef01234567",`, "", 1) + "\n",
				author:  "kunchenguid",
				headRef: "fm/missing-head-sha", headRepo: repo, headSHA: attestationHeadSHA, baseRepo: repo,
			},
			pass: false,
			wantOutput: []string{
				"not bound to the current pull request head",
				"attestation head_sha: (missing)",
			},
			hideOutput: []string{attestationVersionFloor},
		},
		{
			name: "human PR without the signature fails",
			event: pullRequestEvent{
				number: 84, body: "## Summary\n\nA hand-written pull request.", author: "kunchenguid",
				headRef: "fix/something", headRepo: repo, baseRepo: repo,
			},
			pass: false,
		},
		{
			name: "empty body fails",
			event: pullRequestEvent{
				number: 85, body: "", author: "kunchenguid",
				headRef: "fix/something", headRepo: repo, baseRepo: repo,
			},
			pass: false,
		},
		{
			name: "borrowed release branch name alone fails",
			event: pullRequestEvent{
				number: 86, body: "## Summary\n\nNot a release PR.", author: "kunchenguid",
				headRef: "release-please--branches--main", headRepo: repo, baseRepo: repo,
			},
			pass: false,
		},
		{
			name: "fork copying the release body fails",
			event: pullRequestEvent{
				number: 87, body: releaseBody, author: "outside-contributor",
				headRef: "release-please--branches--main", headRepo: forkRepo, baseRepo: repo,
			},
			pass: false,
		},
		{
			name: "release body on an ordinary same-repo branch fails",
			event: pullRequestEvent{
				number: 88, body: releaseBody, author: "kunchenguid",
				headRef: "feat/pretend", headRepo: repo, baseRepo: repo,
			},
			pass: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, output := runGate(t, step, tc.event)
			if got != tc.pass {
				t.Fatalf("gate pass = %v, want %v\n%s", got, tc.pass, output)
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(output, want) {
					t.Fatalf("gate output missing %q\n%s", want, output)
				}
			}
			for _, hide := range tc.hideOutput {
				if strings.Contains(output, hide) {
					t.Fatalf("gate output unexpectedly contains %q\n%s", hide, output)
				}
			}
		})
	}
}

// The required check must always report on a pull request. A path filter would
// silently withhold the status and block the PR forever.
func TestGateWorkflowHasNoPathFilter(t *testing.T) {
	data, err := os.ReadFile(gateWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", gateWorkflowPath, err)
	}
	on, err := parseWorkflowOn(data)
	if err != nil {
		t.Fatalf("parse on: %v", err)
	}
	filter, hasPR, err := pullRequestPathFilter(on)
	if err != nil {
		t.Fatalf("read pull_request filter: %v", err)
	}
	if !hasPR {
		t.Fatal("gate workflow must trigger on pull_request")
	}
	if filter.kind != "none" {
		t.Fatalf("gate workflow must not filter by path, got %s filter %v", filter.kind, filter.patterns)
	}
}
