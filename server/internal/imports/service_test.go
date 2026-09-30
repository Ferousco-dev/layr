package imports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

func okAPI() *fakeFigma {
	return &fakeFigma{
		file:      fileWith(frame("1:2", "Desktop", "FRAME")),
		nodes:     map[string]figma.Node{"1:2": frame("1:2", "Desktop", "FRAME")},
		renderURL: "https://figma-alpha-api.s3.example/render.png",
	}
}

func start(t *testing.T, r *rig, user, project, url string) Import {
	t.Helper()
	imp, err := r.svc.Start(context.Background(), user, project, url)
	if err != nil {
		t.Fatal(err)
	}
	return imp
}

func TestImportWithNodeIDCompletes(t *testing.T) {
	r := newRig(t, okAPI())

	imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
	done := r.settle(t, imp.ID)

	if done.Status != StatusCompleted || done.NodeID != "1:2" || done.NodeName != "Desktop" || done.FileName != "Landing" ||
		done.Version != "77" || done.RenderFormat != "png" || done.CompletedAt == nil || done.ErrorCode != "" {
		t.Fatalf("import = %+v", done)
	}
	snap := readFile(t, r, imp.ID, workspace.Raw, "target-node.json")
	if snap["version"] != "77" {
		t.Fatalf("snapshot = %v", snap)
	}
	render := readFile(t, r, imp.ID, workspace.Reference, "render.json")
	if render["format"] != "png" || fmt.Sprint(render["screens"]) != "[1:2]" {
		t.Fatalf("render = %v", render)
	}
	if _, kept := render["temporary_url"]; kept || strings.Contains(fmt.Sprint(render), "figma-alpha") {
		t.Fatalf("the temporary Figma URL must not be kept: %v", render)
	}
	if r.api.calls.Load() != 2 {
		t.Fatalf("figma calls = %d, want node fetch and render only", r.api.calls.Load())
	}
	for _, user := range r.api.users {
		if user != ownerA {
			t.Fatalf("figma called for user %q", user)
		}
	}
}

func TestImportWithoutNodeIDAutoSelectsSingleFrame(t *testing.T) {
	r := newRig(t, okAPI())

	imp := start(t, r, ownerA, projectA, fileURL)
	done := r.settle(t, imp.ID)

	if done.Status != StatusCompleted || done.NodeID != "1:2" {
		t.Fatalf("import = %+v", done)
	}
	readFile(t, r, imp.ID, workspace.Raw, "file.json")
	readFile(t, r, imp.ID, workspace.Raw, "target-node.json")
}

func TestHiddenAndUnsupportedTopLevelNodesAreNotCandidates(t *testing.T) {
	hidden := frame("1:3", "Hidden", "FRAME")
	no := false
	hidden.Visible = &no
	api := okAPI()
	api.file = fileWith(frame("1:2", "Desktop", "FRAME"), hidden, frame("1:4", "Label", "TEXT"), frame("1:5", "Line", "LINE"))
	r := newRig(t, api)

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)

	if done.Status != StatusCompleted || done.NodeID != "1:2" {
		t.Fatalf("import = %+v", done)
	}
}

func TestMultipleFramesWaitForSelection(t *testing.T) {
	api := okAPI()
	api.file = fileWith(frame("1:2", "Desktop", "FRAME"), frame("1:3", "Mobile", "FRAME"), frame("1:4", "Kit", "SECTION"))
	api.nodes["1:3"] = frame("1:3", "Mobile", "FRAME")
	r := newRig(t, api)

	imp := start(t, r, ownerA, projectA, fileURL)
	waiting := r.settle(t, imp.ID)

	if waiting.Status != StatusAwaitingSelection || len(waiting.Candidates) != 3 || waiting.NodeID != "" {
		t.Fatalf("import = %+v", waiting)
	}
	if api.renders.Load() != 0 {
		t.Fatal("rendered before a frame was chosen")
	}
	if !r.workspaceExists(imp.ID) {
		t.Fatal("workspace must survive while waiting")
	}

	if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, Selection{NodeIDs: []string{"9:9"}}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("unknown node: %v", err)
	}
	if _, err := r.svc.Select(context.Background(), ownerB, projectA, imp.ID, Selection{NodeIDs: []string{"1:3"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign user: %v", err)
	}
	if _, err := r.svc.Select(context.Background(), ownerB, projectB, imp.ID, Selection{NodeIDs: []string{"1:3"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign project: %v", err)
	}
	if got := r.repo.snapshot(imp.ID); got.Status != StatusAwaitingSelection {
		t.Fatalf("rejected selections changed state: %+v", got)
	}

	if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, Selection{NodeIDs: []string{"1:3"}}); err != nil {
		t.Fatal(err)
	}
	done := r.settle(t, imp.ID)
	if done.Status != StatusCompleted || done.NodeID != "1:3" || done.NodeName != "Mobile" || done.Candidates != nil {
		t.Fatalf("import = %+v", done)
	}

	if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, Selection{NodeIDs: []string{"1:2"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("selecting on a completed import: %v", err)
	}
}

func TestSelectingFromAnotherImportsCandidatesIsRejected(t *testing.T) {
	api := okAPI()
	api.file = fileWith(frame("1:2", "Desktop", "FRAME"), frame("1:3", "Mobile", "FRAME"))
	r := newRig(t, api)
	first := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)
	api.file = fileWith(frame("7:1", "Other", "FRAME"), frame("7:2", "Other2", "FRAME"))
	second := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)

	first = r.repo.snapshot(first.ID)
	if first.Status != StatusFailed || first.ErrorCode != CodeSuperseded {
		t.Fatalf("first = %+v", first)
	}
	if _, err := r.svc.Select(context.Background(), ownerA, projectA, second.ID, Selection{NodeIDs: []string{"1:2"}}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("candidate from another import: %v", err)
	}
	if _, err := r.svc.Select(context.Background(), ownerA, projectA, first.ID, Selection{NodeIDs: []string{"1:2"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("superseded import: %v", err)
	}
}

func TestFailuresAreTypedAndCleanedUp(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fakeFigma)
		url    string
		code   string
	}{
		{"no frames", func(f *fakeFigma) { f.file = fileWith(frame("1:4", "Label", "TEXT")) }, fileURL, CodeNoFrames},
		{"node missing", func(f *fakeFigma) { f.nodes = map[string]figma.Node{} }, fileURL + "?node-id=1-2", CodeNodeNotFound},
		{"text root", func(f *fakeFigma) { f.nodes["1:2"] = frame("1:2", "T", "TEXT") }, fileURL + "?node-id=1-2", CodeUnsupportedNode},
		{"future root", func(f *fakeFigma) { f.nodes["1:2"] = frame("1:2", "T", "FUTURE_SUPER_NODE") }, fileURL + "?node-id=1-2", CodeUnsupportedNode},
		{"unauthorized", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindTokenExpired} }, fileURL + "?node-id=1-2", "FIGMA_AUTH_REQUIRED"},
		{"reconnect", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindAuthRequired} }, fileURL + "?node-id=1-2", "FIGMA_AUTH_REQUIRED"},
		{"forbidden", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindPermissionDenied} }, fileURL + "?node-id=1-2", "FIGMA_PERMISSION_DENIED"},
		{"file missing", func(f *fakeFigma) { f.fileErr = &figma.Error{Kind: figma.KindNotFound} }, fileURL, "FIGMA_FILE_NOT_FOUND"},
		{"rate limited", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindRateLimited, RetryAfter: time.Minute} }, fileURL + "?node-id=1-2", "FIGMA_RATE_LIMITED"},
		{"server error", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindUnavailable, Status: 500} }, fileURL + "?node-id=1-2", "FIGMA_UNAVAILABLE"},
		{"figma timeout", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindTimeout} }, fileURL + "?node-id=1-2", "FIGMA_REQUEST_TIMEOUT"},
		{"malformed response", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindBadResponse} }, fileURL + "?node-id=1-2", CodeImportFailed},
		{"bad request", func(f *fakeFigma) { f.nodesErr = &figma.Error{Kind: figma.KindBadRequest} }, fileURL + "?node-id=1-2", CodeImportFailed},
		{"render failed", func(f *fakeFigma) { f.renderErr = &figma.Error{Kind: figma.KindRateLimited} }, fileURL + "?node-id=1-2", "FIGMA_RATE_LIMITED"},
		{"render empty", func(f *fakeFigma) { f.renderURL = "" }, fileURL + "?node-id=1-2", CodeImportFailed},
		{"unknown error", func(f *fakeFigma) { f.nodesErr = errors.New("SECRET_TOKEN leaked in cause") }, fileURL + "?node-id=1-2", CodeImportFailed},
	}
	for _, tc := range cases {
		api := okAPI()
		tc.mutate(api)
		r := newRig(t, api)

		imp := start(t, r, ownerA, projectA, tc.url)
		done := r.settle(t, imp.ID)

		if done.Status != StatusFailed || done.ErrorCode != tc.code {
			t.Errorf("%s: status %s code %q, want %q", tc.name, done.Status, done.ErrorCode, tc.code)
		}
		if r.workspaceExists(imp.ID) {
			t.Errorf("%s: workspace not cleaned after failure", tc.name)
		}
		if strings.Contains(r.logs.String(), "SECRET_TOKEN") && tc.name != "unknown error" {
			t.Errorf("%s: secret in logs", tc.name)
		}
		if api.calls.Load() > 3 {
			t.Errorf("%s: %d figma calls; failures must not loop", tc.name, api.calls.Load())
		}
	}
}

func TestRateLimitIsNotRetriedByTheImporter(t *testing.T) {
	api := okAPI()
	api.nodesErr = &figma.Error{Kind: figma.KindRateLimited, RetryAfter: 30 * time.Second, Retryable: true}
	r := newRig(t, api)

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.ErrorCode != "FIGMA_RATE_LIMITED" || api.calls.Load() != 1 || !strings.Contains(r.logs.String(), "retry_after_s=30") {
		t.Fatalf("code %q calls %d logs %s", done.ErrorCode, api.calls.Load(), r.logs.String())
	}
}

func TestCancellationStopsWorkAndFailsTheImport(t *testing.T) {
	api := okAPI()
	api.block = make(chan struct{})
	r := newRig(t, api)
	imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
	waitUntil(t, func() bool { return api.calls.Load() >= 1 })

	r.svc.Close(context.Background())

	done := r.repo.snapshot(imp.ID)
	if done.Status != StatusFailed || done.ErrorCode != CodeCancelled {
		t.Fatalf("import = %+v", done)
	}
	if api.renders.Load() != 0 || r.workspaceExists(imp.ID) {
		t.Fatal("work continued or workspace left behind after cancellation")
	}
}

func TestOverallTimeoutFailsTheImport(t *testing.T) {
	api := okAPI()
	api.block = make(chan struct{})
	r := newRig(t, api, func(c *Config) { c.Timeout = 50 * time.Millisecond })

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.Status != StatusFailed || done.ErrorCode != CodeTimeout {
		t.Fatalf("import = %+v", done)
	}
}

func TestOwnershipIsCheckedBeforeAnyWork(t *testing.T) {
	api := okAPI()
	r := newRig(t, api)

	_, err := r.svc.Start(context.Background(), ownerB, projectA, fileURL)

	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("err = %v", err)
	}
	if api.calls.Load() != 0 || len(r.repo.rows) != 0 {
		t.Fatal("work started for a project the user does not own")
	}
	entries, _ := os.ReadDir(filepath.Join(r.root, "imports"))
	if len(entries) != 0 {
		t.Fatal("workspace created for a foreign project")
	}

	imp := start(t, r, ownerA, projectA, fileURL)
	r.settle(t, imp.ID)
	if _, err := r.svc.Get(context.Background(), ownerB, projectA, imp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	if _, err := r.svc.Latest(context.Background(), ownerB, projectA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign latest: %v", err)
	}
	if _, err := r.svc.Get(context.Background(), ownerA, projectB, imp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project read: %v", err)
	}
}

func TestInvalidInputsNeverReachStorageOrFigma(t *testing.T) {
	api := okAPI()
	r := newRig(t, api)

	for _, url := range []string{"", "https://evil.example/design/" + fileKey, "http://www.figma.com/design/" + fileKey, "file:///etc/passwd"} {
		if _, err := r.svc.Start(context.Background(), ownerA, projectA, url); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q: %v", url, err)
		}
	}
	if _, err := r.svc.Start(context.Background(), ownerA, "not-a-uuid", fileURL); !errors.Is(err, ErrInvalidID) {
		t.Errorf("project id: %v", err)
	}
	if _, err := r.svc.Get(context.Background(), ownerA, projectA, "nope"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("import id: %v", err)
	}
	if r.repo.createN.Load() != 0 || api.calls.Load() != 0 {
		t.Fatal("invalid input reached storage or Figma")
	}
}

func TestConcurrentStartsAllowExactlyOne(t *testing.T) {
	api := okAPI()
	api.block = make(chan struct{})
	r := newRig(t, api, func(c *Config) { c.MaxRunning = 50 })

	var wg sync.WaitGroup
	var mu sync.Mutex
	won, conflicts := 0, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.svc.Start(context.Background(), ownerA, projectA, fileURL+"?node-id=1-2")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, ErrConflict):
				conflicts++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	close(api.block)

	if won != 1 || conflicts != 19 {
		t.Fatalf("won %d conflicts %d", won, conflicts)
	}
}

func TestNewImportAllowedAfterPreviousFinishesAndHistoryIsKept(t *testing.T) {
	r := newRig(t, okAPI())
	first := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)
	second := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)

	latest, _ := r.svc.Latest(context.Background(), ownerA, projectA)
	if first.Status != StatusCompleted || second.Status != StatusCompleted || latest.ID != second.ID || len(r.repo.rows) != 2 {
		t.Fatalf("first %s second %s latest %s", first.Status, second.Status, latest.ID)
	}
}

func TestStaleRunningImportDoesNotBlockTheProject(t *testing.T) {
	r := newRig(t, okAPI())
	r.repo.rows = append(r.repo.rows, &Import{
		ID: "dddddddd-0000-4000-8000-000000000001", ProjectID: projectA, Status: StatusProcessing,
		CreatedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now().Add(-time.Hour),
	})

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)

	if done.Status != StatusCompleted || r.repo.rows[0].ErrorCode != CodeSuperseded {
		t.Fatalf("done %s, old %+v", done.Status, r.repo.rows[0])
	}
}

func TestBusyImporterRejectsInsteadOfQueueingForever(t *testing.T) {
	api := okAPI()
	api.block = make(chan struct{})
	r := newRig(t, api, func(c *Config) { c.MaxRunning = 1 })
	start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")

	_, err := r.svc.Start(context.Background(), ownerB, projectB, fileURL)

	close(api.block)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v", err)
	}
}

func TestOversizedSnapshotFailsSafely(t *testing.T) {
	r := newRig(t, okAPI())
	mgr, _ := workspace.NewManager(t.TempDir(), 10, 100)
	r.svc.ws = mgr

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.ErrorCode != CodeSnapshotTooLarge {
		t.Fatalf("import = %+v", done)
	}
}

func TestPanicInsideAnImportFailsThatImportOnly(t *testing.T) {
	api := okAPI()
	api.panicNodes = true
	r := newRig(t, api)

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.Status != StatusFailed || done.ErrorCode != CodeInternal {
		t.Fatalf("import = %+v", done)
	}
	if strings.Contains(done.ErrorCode, "SECRET") {
		t.Fatal("panic text stored")
	}
	api.panicNodes = false
	if again := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID); again.Status != StatusCompleted {
		t.Fatal("service unusable after a panic")
	}
}

func TestSweepRemovesOnlyStaleIdleWorkspaces(t *testing.T) {
	r := newRig(t, okAPI())
	done := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(filepath.Join(r.root, "imports", done.ID), old, old)
	runningID := "eeeeeeee-0000-4000-8000-000000000001"
	_, _ = r.mgr.Create(runningID)
	_ = os.Chtimes(filepath.Join(r.root, "imports", runningID), old, old)
	r.repo.rows = append(r.repo.rows, &Import{ID: runningID, ProjectID: projectB, Status: StatusProcessing, UpdatedAt: time.Now()})

	r.svc.Sweep(context.Background())

	if r.workspaceExists(done.ID) || !r.workspaceExists(runningID) {
		t.Fatal("sweep removed the wrong workspaces")
	}
}

func TestSweepFailsImportsAbandonedByACrashedProcess(t *testing.T) {
	r := newRig(t, okAPI())
	old := time.Now().Add(-time.Hour)
	r.repo.rows = append(r.repo.rows,
		&Import{ID: "eeeeeeee-0000-4000-8000-000000000001", ProjectID: projectA, Status: StatusProcessing, UpdatedAt: old},
		&Import{ID: "eeeeeeee-0000-4000-8000-000000000002", ProjectID: projectB, Status: StatusPending, UpdatedAt: old},
		&Import{ID: "eeeeeeee-0000-4000-8000-000000000003", ProjectID: projectB, Status: StatusProcessing, UpdatedAt: time.Now()},
		&Import{ID: "eeeeeeee-0000-4000-8000-000000000004", ProjectID: projectB, Status: StatusCompleted, UpdatedAt: old},
	)

	r.svc.Sweep(context.Background())

	for id, want := range map[string]string{
		"eeeeeeee-0000-4000-8000-000000000001": StatusFailed,
		"eeeeeeee-0000-4000-8000-000000000002": StatusFailed,
		"eeeeeeee-0000-4000-8000-000000000003": StatusProcessing,
		"eeeeeeee-0000-4000-8000-000000000004": StatusCompleted,
	} {
		if got := r.repo.snapshot(id); got.Status != want {
			t.Errorf("%s = %s, want %s", id, got.Status, want)
		}
	}
	if got := r.repo.snapshot("eeeeeeee-0000-4000-8000-000000000001"); got.ErrorCode != CodeInterrupted || Message(got.ErrorCode) == Message("INTERNAL_ERROR") {
		t.Fatalf("abandoned import = %+v", got)
	}
}

func TestSnapshotsNeverContainCredentials(t *testing.T) {
	r := newRig(t, okAPI())
	imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
	r.settle(t, imp.ID)

	var found []string
	_ = filepath.Walk(filepath.Join(r.root, "imports", imp.ID), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(path)
			if strings.Contains(strings.ToLower(string(b)), "bearer") || strings.Contains(strings.ToLower(string(b)), "access_token") {
				found = append(found, path)
			}
			if info.Mode().Perm()&0o077 != 0 {
				t.Errorf("%s is readable by others", path)
			}
		}
		return nil
	})
	if len(found) != 0 {
		t.Fatalf("credentials found in %v", found)
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func withAssets(t *testing.T, api *fakeFigma, a *fakeAssets) *rig {
	t.Helper()
	r := newRig(t, api)
	r.svc.SetAssets(a)
	return r
}

func TestAssetsAreDownloadedFromTheTargetSubtreeBeforeCompletion(t *testing.T) {
	api := okAPI()
	api.nodes["1:2"] = figma.Node{ID: "1:2", Name: "Desktop", Type: "FRAME", Children: []figma.Node{{ID: "1:9", Name: "Logo", Type: "VECTOR"}}}
	pipeline := &fakeAssets{summary: assets.Summary{Assets: 3, Warnings: 1}}
	r := withAssets(t, api, pipeline)

	imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
	done := r.settle(t, imp.ID)

	if done.Status != StatusCompleted || done.AssetCount != 3 || done.WarningCount != 1 {
		t.Fatalf("import = %+v", done)
	}
	if pipeline.referenceURL["1:2"] != "https://figma-alpha-api.s3.example/render.png" || pipeline.runs.Load() != 1 {
		t.Fatalf("reference urls %v runs %d", pipeline.referenceURL, pipeline.runs.Load())
	}
	in := pipeline.input
	if in.ImportID != imp.ID || in.UserID != ownerA || in.FileKey != fileKey || len(in.NodeIDs) != 1 || in.NodeIDs[0] != "1:2" ||
		len(in.Root.Children) != 1 || len(in.Root.Children[0].Children) != 1 || in.Root.Children[0].Children[0].Name != "Logo" {
		t.Fatalf("pipeline input = %+v", in)
	}
	if len(in.References) != 1 || in.References[0].Path != "reference/1-2.png" {
		t.Fatalf("references not passed to the manifest: %+v", in.References)
	}
	if _, err := os.Stat(filepath.Join(r.root, "imports", imp.ID, "reference", "1-2.png")); err != nil {
		t.Fatal("reference image missing")
	}
}

func TestAssetFailuresFailTheImportWithTheirOwnCodes(t *testing.T) {
	cases := map[string]struct {
		ref, run error
		code     string
	}{
		"too large":      {run: &assets.Error{Code: assets.CodeTooLarge}, code: "ASSET_TOO_LARGE"},
		"budget":         {run: &assets.Error{Code: assets.CodeBudgetExceeded}, code: "ASSET_BUDGET_EXCEEDED"},
		"invalid":        {run: &assets.Error{Code: assets.CodeInvalidContent}, code: "ASSET_INVALID_CONTENT"},
		"blocked":        {run: &assets.Error{Code: assets.CodeURLBlocked}, code: "ASSET_URL_BLOCKED"},
		"download":       {run: &assets.Error{Code: assets.CodeDownloadFailed}, code: "ASSET_DOWNLOAD_FAILED"},
		"figma limit":    {run: &figma.Error{Kind: figma.KindRateLimited}, code: "FIGMA_RATE_LIMITED"},
		"reference fail": {ref: &assets.Error{Code: assets.CodeInvalidContent}, code: "ASSET_INVALID_CONTENT"},
		"unknown":        {run: errors.New("SECRET_TOKEN boom"), code: CodeImportFailed},
	}
	for name, tc := range cases {
		r := withAssets(t, okAPI(), &fakeAssets{referenceErr: tc.ref, runErr: tc.run})

		imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
		done := r.settle(t, imp.ID)

		if done.Status != StatusFailed || done.ErrorCode != tc.code {
			t.Errorf("%s: import = %+v", name, done)
		}
		if r.workspaceExists(imp.ID) {
			t.Errorf("%s: workspace kept after failure", name)
		}
	}
}

func TestAssetsAreNotFetchedForAnUnimportableTarget(t *testing.T) {
	api := okAPI()
	api.nodes["1:2"] = frame("1:2", "T", "TEXT")
	pipeline := &fakeAssets{}
	r := withAssets(t, api, pipeline)

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.ErrorCode != CodeUnsupportedNode || pipeline.runs.Load() != 0 || api.renders.Load() != 0 {
		t.Fatalf("import = %+v runs %d renders %d", done, pipeline.runs.Load(), api.renders.Load())
	}
}

func TestCancellationDuringAssetWorkFailsTheImport(t *testing.T) {
	pipeline := &fakeAssets{runErr: context.Canceled}
	r := withAssets(t, okAPI(), pipeline)
	imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
	waitUntil(t, func() bool { return pipeline.runs.Load() > 0 })

	r.svc.Close(context.Background())

	done := r.repo.snapshot(imp.ID)
	if done.Status != StatusFailed || r.workspaceExists(imp.ID) {
		t.Fatalf("import = %+v", done)
	}
}

func TestFullImportWithTheRealAssetPipeline(t *testing.T) {
	png := pngFixture()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reference.png", "/hero.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>x()</script><path d="M0 0L9 9"/></svg>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer cdn.Close()
	api := okAPI()
	api.renderURL = cdn.URL + "/reference.png"
	api.nodes["1:2"] = figma.Node{ID: "1:2", Name: "Desktop", Type: "FRAME", Children: []figma.Node{
		{ID: "1:3", Name: "Hero", Type: "RECTANGLE", Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "heroref"}}},
		{ID: "1:4", Name: "Logo", Type: "VECTOR"},
	}}
	api.fills = map[string]string{"heroref": cdn.URL + "/hero.png"}
	api.renderMap = map[string]string{"1:4": cdn.URL + "/logo.svg"}
	r := newRig(t, api)
	r.svc.SetAssets(assets.NewPipeline(api, assets.NewDownloader(assets.Policy{AllowInsecure: true}, 1<<20), nil,
		assets.Config{MaxAssetBytes: 1 << 20, MaxImportBytes: 4 << 20, Concurrency: 2, MaxAssets: 50}))
	r.svc.SetDesign(DesignConfig{MaxNodes: 10000, MaxBytes: 4 << 20})

	imp := start(t, r, ownerA, projectA, fileURL+"?node-id=1-2")
	done := r.settle(t, imp.ID)

	if done.Status != StatusCompleted || done.AssetCount != 2 || done.ScreenCount != 1 || done.DesignNodes != 3 {
		t.Fatalf("import = %+v", done)
	}
	dir, _ := r.mgr.Create(imp.ID)
	if ref, err := dir.ReadFile(workspace.Reference, "1-2-"+refSuffix("1:2")+".png"); err != nil || len(ref) != len(png) {
		t.Fatalf("reference image: %v", err)
	}
	irData, err := dir.ReadFile(workspace.Design, "design-ir.json")
	if err != nil {
		t.Fatal(err)
	}
	ir, err := designir.Unmarshal(irData, designir.DefaultLimits)
	if err != nil || len(ir.Screens) != 1 || ir.Screens[0].Reference == nil || len(ir.Assets) != 2 || ir.Source.ImportID != imp.ID {
		t.Fatalf("design ir = %+v err %v", ir, err)
	}
	for _, bad := range []string{cdn.URL, "http://", r.root} {
		if strings.Contains(string(irData), bad) {
			t.Fatalf("design contains %q", bad)
		}
	}
	names, _ := dir.Names(workspace.Assets)
	if len(names) != 3 {
		t.Fatalf("assets dir = %v", names)
	}
	m, err := assets.LoadManifest(dir)
	if err != nil || len(m.References) != 1 || !strings.HasPrefix(m.References[0].Path, "reference/1-2-") || len(m.Assets) != 2 {
		t.Fatalf("manifest = %+v err %v", m, err)
	}
	logo := m.AssetsForNode("1:4")[0]
	stored, _ := dir.ReadFile(workspace.Assets, strings.TrimPrefix(logo.Path, "assets/"))
	if strings.Contains(string(stored), "script") || !logo.Sanitized {
		t.Fatalf("logo not sanitized: %s", stored)
	}
	render := readFile(t, r, imp.ID, workspace.Reference, "render.json")
	if strings.Contains(fmt.Sprint(render), cdn.URL) {
		t.Fatalf("temporary URL kept: %v", render)
	}
}

func refSuffix(nodeID string) string {
	sum := sha256.Sum256([]byte(nodeID))
	return hex.EncodeToString(sum[:])[:6]
}

func pngFixture() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2)))
	return b.Bytes()
}

func multiScreenAPI() *fakeFigma {
	api := okAPI()
	api.file = fileWith(frame("1:2", "Desktop", "FRAME"), frame("1:3", "Mobile", "FRAME"), frame("1:4", "Flow", "SECTION"))
	api.file.Document.Children[0].Name = "Page One"
	api.nodes["1:2"] = figma.Node{ID: "1:2", Name: "Desktop", Type: "FRAME", Children: []figma.Node{{ID: "9:1", Name: "Btn", Type: "INSTANCE", ComponentID: "5:100"}}}
	api.nodes["1:3"] = figma.Node{ID: "1:3", Name: "Mobile", Type: "FRAME", Children: []figma.Node{{ID: "9:2", Name: "Btn", Type: "INSTANCE", ComponentID: "5:100"}}}
	api.nodes["1:4"] = figma.Node{ID: "1:4", Name: "Flow", Type: "SECTION", Children: []figma.Node{
		{ID: "4:1", Name: "Step 1", Type: "FRAME"}, {ID: "4:2", Name: "Step 2", Type: "FRAME"},
	}}
	api.components = map[string]figma.ComponentMeta{"5:100": {Key: "k", Name: "Button"}}
	return api
}

func TestSelectingEverythingImportsEveryScreenAndSectionFrames(t *testing.T) {
	api := multiScreenAPI()
	pipeline := &fakeAssets{}
	r := withAssets(t, api, pipeline)
	r.svc.SetDesign(DesignConfig{MaxNodes: 10000, MaxBytes: 4 << 20})

	imp := start(t, r, ownerA, projectA, fileURL)
	waiting := r.settle(t, imp.ID)
	if waiting.Status != StatusAwaitingSelection || waiting.Candidates[0].Page != "Page One" {
		t.Fatalf("waiting = %+v", waiting)
	}
	if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, Selection{All: true}); err != nil {
		t.Fatal(err)
	}
	done := r.settle(t, imp.ID)

	if done.Status != StatusCompleted || len(done.NodeIDs) != 3 || done.ScreenCount != 4 {
		t.Fatalf("import = %+v", done)
	}
	if len(pipeline.referenceURL) != 4 || pipeline.input.NodeIDs[3] != "4:2" || len(pipeline.input.Root.Children) != 4 {
		t.Fatalf("the reference and asset steps must cover all four screens: %v %v", pipeline.referenceURL, pipeline.input.NodeIDs)
	}
	if api.renders.Load() != 1 {
		t.Fatalf("all screens must be rendered in one Figma request, got %d", api.renders.Load())
	}
	dir, _ := r.mgr.Create(imp.ID)
	data, err := dir.ReadFile(workspace.Design, "design-ir.json")
	if err != nil {
		t.Fatal(err)
	}
	ir, err := designir.Unmarshal(data, designir.DefaultLimits)
	if err != nil || len(ir.Screens) != 4 || len(ir.Sections) != 1 || len(ir.Components) != 1 || ir.Components[0].InstanceCount != 2 || len(ir.Components[0].ScreenIDs) != 2 {
		t.Fatalf("design = %+v err %v", ir.Stats, err)
	}
	one, err := ir.Select([]string{ir.Screens[1].ID}, designir.DefaultLimits)
	if err != nil || len(one.Screens) != 1 || one.Components[0].InstanceCount != 1 {
		t.Fatalf("a screen can be taken out without re-importing: %v", err)
	}
}

func TestSelectingSomeScreensImportsOnlyThose(t *testing.T) {
	api := multiScreenAPI()
	pipeline := &fakeAssets{}
	r := withAssets(t, api, pipeline)
	imp := start(t, r, ownerA, projectA, fileURL)
	r.settle(t, imp.ID)

	if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, Selection{NodeIDs: []string{"1:3", "1:2"}}); err != nil {
		t.Fatal(err)
	}
	done := r.settle(t, imp.ID)

	if done.ScreenCount != 2 || done.NodeIDs[0] != "1:2" || done.NodeIDs[1] != "1:3" || len(pipeline.referenceURL) != 2 {
		t.Fatalf("import = %+v", done)
	}
}

func TestMoreScreensThanTheLimitFailTheImport(t *testing.T) {
	api := multiScreenAPI()
	r := newRig(t, api, func(c *Config) { c.MaxScreens = 3 })
	imp := start(t, r, ownerA, projectA, fileURL)
	r.settle(t, imp.ID)

	if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, Selection{All: true}); err != nil {
		t.Fatal(err)
	}
	done := r.settle(t, imp.ID)

	if done.Status != StatusFailed || done.ErrorCode != CodeTooManyScreens || api.renders.Load() != 0 {
		t.Fatalf("import = %+v renders %d", done, api.renders.Load())
	}
}

func TestSelectionValidationHappensBeforeAnyWork(t *testing.T) {
	r := newRig(t, multiScreenAPI(), func(c *Config) { c.MaxScreens = 3 })
	imp := start(t, r, ownerA, projectA, fileURL)
	r.settle(t, imp.ID)

	for name, sel := range map[string]Selection{
		"empty": {}, "too many ids": {NodeIDs: []string{"1:2", "1:3", "1:4", "1:5"}}, "duplicate": {NodeIDs: []string{"1:2", "1:2"}}, "unknown": {NodeIDs: []string{"9:9"}},
	} {
		if _, err := r.svc.Select(context.Background(), ownerA, projectA, imp.ID, sel); !errors.Is(err, ErrInvalidSelection) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := r.repo.snapshot(imp.ID); got.Status != StatusAwaitingSelection {
		t.Fatalf("import = %+v", got)
	}
}

func TestDesignFailuresFailTheImportWithTheirCodes(t *testing.T) {
	r := newRig(t, okAPI())
	r.svc.SetDesign(DesignConfig{MaxNodes: 10000, MaxBytes: 10})

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.Status != StatusFailed || done.ErrorCode != designir.CodeLimit || r.workspaceExists(done.ID) {
		t.Fatalf("import = %+v", done)
	}
}

func TestRefreshImportsTheSameFileAgainAndNeedsAFinishedImport(t *testing.T) {
	r := newRig(t, okAPI())
	if _, err := r.svc.Refresh(context.Background(), ownerA, projectA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nothing imported yet: %v", err)
	}

	first := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)
	if first.Status != StatusCompleted {
		t.Fatalf("first = %+v", first)
	}
	again, err := r.svc.Refresh(context.Background(), ownerA, projectA)
	if err != nil || again.ID == first.ID || again.FileKey != first.FileKey || again.Status != StatusPending {
		t.Fatalf("refresh = %+v, %v", again, err)
	}
	if done := r.settle(t, again.ID); done.Status != StatusCompleted && done.Status != StatusAwaitingSelection {
		t.Fatalf("the refreshed import = %+v", done)
	}

	if _, err := r.svc.Refresh(context.Background(), ownerB, projectA); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("another person must not refresh this project: %v", err)
	}
	if _, err := r.svc.Refresh(context.Background(), ownerA, "not-a-uuid"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("bad id: %v", err)
	}
}

func TestLinkToAPageOrGroupOffersTheFramesAroundIt(t *testing.T) {
	for name, link := range map[string]struct{ id, typ, node string }{
		"page":  {"0:1", "CANVAS", "0-1"},
		"group": {"1:9", "GROUP", "1-9"},
	} {
		api := okAPI()
		api.file = fileWith(frame("1:2", "Desktop", "FRAME"), frame("1:3", "Mobile", "FRAME"), frame("1:9", "Group", "GROUP"))
		api.nodes[link.id] = frame(link.id, "Container", link.typ)
		r := newRig(t, api)

		imp := start(t, r, ownerA, projectA, fileURL+"?node-id="+link.node)
		waiting := r.settle(t, imp.ID)

		if waiting.Status != StatusAwaitingSelection || len(waiting.Candidates) != 2 || waiting.ErrorCode != "" {
			t.Fatalf("%s link: %+v", name, waiting)
		}
		if api.renders.Load() != 0 {
			t.Fatalf("%s link rendered before a choice", name)
		}
	}
}

func TestLinkToAPageWithOneFrameImportsIt(t *testing.T) {
	api := okAPI()
	api.nodes["0:1"] = frame("0:1", "Page", "CANVAS")
	r := newRig(t, api)

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=0-1").ID)

	if done.Status != StatusCompleted || done.ScreenCount != 1 {
		t.Fatalf("import = %+v", done)
	}
}

func TestMoreThanOneHundredScreensAreFetchedInBatches(t *testing.T) {
	api := okAPI()
	var frames []figma.Node
	api.nodes = map[string]figma.Node{}
	for i := 0; i < 230; i++ {
		id := fmt.Sprintf("2:%d", i)
		frames = append(frames, frame(id, fmt.Sprintf("Screen %d", i), "FRAME"))
		api.nodes[id] = frame(id, fmt.Sprintf("Screen %d", i), "FRAME")
	}
	api.file = fileWith(frames...)
	r := newRig(t, api)

	waiting := r.settle(t, start(t, r, ownerA, projectA, fileURL).ID)
	if waiting.Status != StatusAwaitingSelection || len(waiting.Candidates) != 230 {
		t.Fatalf("candidates = %d, status %s", len(waiting.Candidates), waiting.Status)
	}
	if _, err := r.svc.Select(context.Background(), ownerA, projectA, waiting.ID, Selection{All: true}); err != nil {
		t.Fatal(err)
	}
	done := r.settle(t, waiting.ID)

	if done.Status != StatusCompleted || done.ScreenCount != 230 {
		t.Fatalf("import = %+v", done)
	}
}
