package designapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/designapi/designtest"
	"github.com/ferousco-dev/layr/server/internal/imports"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

var ctx = context.Background()

func TestSummaryListsScreensFlowsWarningsAndCounts(t *testing.T) {
	e := designtest.New(t)

	d, err := e.Service(100).Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}

	if d.Status != "ready" || d.ProjectID != designtest.ProjectA || d.ImportID != designtest.ImportID || !strings.HasPrefix(d.DesignVersion, "dv_") {
		t.Fatalf("design = %+v", d)
	}
	if d.Source.FileName != "SaaS Dashboard" || d.Source.SourceVersion != "77" || d.Source.Provider != "figma" {
		t.Fatalf("source = %+v", d.Source)
	}
	names := []string{}
	for _, s := range d.Screens {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != strings.Join(designtest.Names, ",") {
		t.Fatalf("order = %v, want the design order", names)
	}
	if d.Screens[0].FlowID != "" || d.Screens[1].FlowID != designtest.SectionID(1) || d.Screens[6].FlowID != designtest.SectionID(2) {
		t.Fatalf("flow membership: %+v", d.Screens)
	}
	if len(d.Flows) != 2 || d.Flows[0].Name != "Authentication" || len(d.Flows[0].ScreenIDs) != 3 || d.Flows[1].Name != "Dashboard" {
		t.Fatalf("flows = %+v", d.Flows)
	}
	c := d.Counts
	if c.Screens != 7 || c.Flows != 2 || c.UngroupedScreens != 1 || c.Warnings != 2 || c.ScreensWithWarnings != 1 || c.SharedComponents != 1 {
		t.Fatalf("counts = %+v", c)
	}
	if strings.Join(d.Selection.Modes, ",") != "one,selected,flow,all" {
		t.Fatalf("modes = %v", d.Selection.Modes)
	}
	p := d.Screens[1].Preview
	if !p.Available || p.MediaType != "image/png" || p.URL != "/api/v1/projects/"+designtest.ProjectA+"/design/screens/"+designtest.ScreenID(2)+"/preview?v="+d.DesignVersion {
		t.Fatalf("preview = %+v", p)
	}
}

func TestWarningsUseFixedPublicTextsOnly(t *testing.T) {
	e := designtest.New(t)

	d, _ := e.Service(100).Design(ctx, designtest.OwnerA, designtest.ProjectA)

	if len(d.Warnings) != 2 {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	byCode := map[string]designapi.Warning{}
	for _, w := range d.Warnings {
		byCode[w.Code] = w
	}
	if w := byCode["UNSUPPORTED_PAINT"]; w.ScreenID != designtest.ScreenID(6) || !strings.Contains(w.Message, "does not fully support") {
		t.Fatalf("warning = %+v", w)
	}
	if _, ok := byCode["DESIGN_NOT_FULLY_SUPPORTED"]; !ok {
		t.Fatalf("an unknown code must fold into a generic public one: %+v", d.Warnings)
	}
	raw, _ := json.Marshal(d)
	for _, leak := range []string{"internal wording", "/tmp/secret", "SOMETHING_NEW", "another internal message"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("IR text %q reached the DTO", leak)
		}
	}
}

func TestDTOsNeverContainInternalDetail(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	d, _ := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
	detail, _ := svc.Screen(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(2))
	page, _ := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{})
	flow, _ := svc.Flow(ctx, designtest.OwnerA, designtest.ProjectA, designtest.SectionID(1))

	for name, v := range map[string]any{"design": d, "detail": detail, "page": page, "flow": flow} {
		raw, _ := json.Marshal(v)
		text := string(raw)
		for _, bad := range []string{
			e.Root, "/tmp", "/Users", "reference/", "assets/", "raw/", ".png", "https://", "http://", "sha256", "signed", "token", "Bearer",
			`"root"`, `"children"`, `"geometry"`, "FILEKEY123456", "file_key", "workspace",
		} {
			if strings.Contains(text, bad) {
				t.Errorf("%s DTO contains %q: %.200s", name, bad, text)
			}
		}
	}
}

func TestScreenDetailIsSmallAndUseful(t *testing.T) {
	e := designtest.New(t)

	d, err := e.Service(100).Screen(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(6))
	if err != nil {
		t.Fatal(err)
	}

	if d.Name != "Analytics" || d.FlowName != "Dashboard" || d.NodeCount != 2 || d.RootLayout != "vertical" || d.WarningCount != 1 || len(d.Warnings) != 1 {
		t.Fatalf("detail = %+v", d)
	}
	if len(d.Components) != 1 || !d.Components[0].Shared || d.Components[0].InstanceCount != 7 || d.Components[0].Name != "Button" {
		t.Fatalf("components = %+v", d.Components)
	}
	if _, err := e.Service(100).Screen(ctx, designtest.OwnerA, designtest.ProjectA, "screen_00000000000000ff"); !errors.Is(err, designapi.ErrScreenNotFound) {
		t.Fatalf("unknown screen: %v", err)
	}
}

func TestFlowDetailAndFilteredScreenList(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)

	f, err := svc.Flow(ctx, designtest.OwnerA, designtest.ProjectA, designtest.SectionID(1))
	if err != nil || f.Name != "Authentication" || len(f.Screens) != 3 || f.Screens[2].Name != "Forgot Password" {
		t.Fatalf("flow = %+v err %v", f, err)
	}
	if _, err := svc.Flow(ctx, designtest.OwnerA, designtest.ProjectA, designtest.SectionID(9)); !errors.Is(err, designapi.ErrFlowNotFound) {
		t.Fatalf("unknown flow: %v", err)
	}
	page, err := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{FlowID: designtest.SectionID(2)})
	if err != nil || page.Total != 3 || page.Screens[0].Name != "Overview" {
		t.Fatalf("filtered = %+v err %v", page, err)
	}
	page, _ = svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{Search: "  UP  "})
	if page.Total != 1 || page.Screens[0].Name != "Signup" {
		t.Fatalf("search = %+v", page)
	}
	if _, err := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{FlowID: designtest.SectionID(9)}); !errors.Is(err, designapi.ErrFlowNotFound) {
		t.Fatalf("unknown flow filter: %v", err)
	}
}

func TestOneScreenProjectWorksWithoutFlows(t *testing.T) {
	e := designtest.NewWith(t, designtest.BuildIR(1, false))

	d, err := e.Service(100).Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Screens) != 1 || len(d.Flows) != 0 || d.Counts.UngroupedScreens != 1 || strings.Join(d.Selection.Modes, ",") != "one,selected,all" {
		t.Fatalf("design = %+v", d)
	}
	if d.Flows == nil || d.Warnings == nil {
		t.Fatal("empty lists must be [] not null")
	}
}

func TestLargeDesignsStayBoundedAndPageDeterministically(t *testing.T) {
	e := designtest.NewWith(t, designtest.BuildIR(520, false))
	svc := e.Service(1000)

	d, err := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Screens) != 500 || !d.ScreensTruncated || d.Counts.Screens != 520 {
		t.Fatalf("summary: %d screens, truncated %v, total %d", len(d.Screens), d.ScreensTruncated, d.Counts.Screens)
	}

	var seen []string
	for offset := 0; ; offset += 100 {
		page, err := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{Limit: 100, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 520 {
			t.Fatalf("total = %d", page.Total)
		}
		for _, s := range page.Screens {
			seen = append(seen, s.ID)
		}
		if len(page.Screens) < 100 {
			break
		}
	}
	if len(seen) != 520 || seen[0] != designtest.ScreenID(1) || seen[519] != designtest.ScreenID(520) {
		t.Fatalf("paging lost or reordered screens: %d", len(seen))
	}
	for _, q := range []designapi.Query{{Limit: 501}, {Limit: -1}, {Offset: -1}, {Search: strings.Repeat("x", 101)}} {
		if _, err := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, q); !errors.Is(err, designapi.ErrInvalidQuery) {
			t.Errorf("%+v accepted: %v", q, err)
		}
	}
	if page, _ := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{Offset: 9999}); len(page.Screens) != 0 || page.Total != 520 {
		t.Fatalf("offset past the end: %+v", page)
	}
}

func TestEveryLookupIsScopedToTheOwner(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	stranger := designtest.OwnerB

	for name, err := range map[string]error{
		"design":  second(svc.Design(ctx, stranger, designtest.ProjectA)),
		"screens": second(svc.Screens(ctx, stranger, designtest.ProjectA, designapi.Query{})),
		"screen":  second(svc.Screen(ctx, stranger, designtest.ProjectA, designtest.ScreenID(1))),
		"flow":    second(svc.Flow(ctx, stranger, designtest.ProjectA, designtest.SectionID(1))),
		"preview": second(svc.Preview(ctx, stranger, designtest.ProjectA, designtest.ScreenID(1))),
	} {
		if !errors.Is(err, designapi.ErrProjectNotFound) {
			t.Errorf("%s: err = %v, want the same not-found as a missing project", name, err)
		}
	}
	if _, err := svc.Design(ctx, designtest.OwnerA, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, designapi.ErrProjectNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	if _, err := svc.Design(ctx, designtest.OwnerA, "not-a-uuid"); !errors.Is(err, designapi.ErrProjectNotFound) {
		t.Fatalf("malformed project: %v", err)
	}
	if _, err := svc.Design(ctx, designtest.OwnerB, designtest.ProjectB); !errors.Is(err, designapi.ErrDesignNotFound) {
		t.Fatalf("B's own project without imports: %v", err)
	}
}

func second[T any](_ T, err error) error { return err }

func TestImportStatesBeforeAndBesideTheCurrentDesign(t *testing.T) {
	newEnv := func() *designtest.Env {
		e := designtest.New(t)
		e.Imports.Rows = nil
		return e
	}
	add := func(e *designtest.Env, id, status, code string) {
		e.Imports.Add(imports.Import{ID: id, ProjectID: designtest.ProjectA, FileName: "SaaS Dashboard", Status: status, ErrorCode: code, ScreenCount: 7})
	}

	for status, want := range map[string]string{
		imports.StatusPending: "importing", imports.StatusProcessing: "importing", imports.StatusAwaitingSelection: "awaiting_selection",
	} {
		e := newEnv()
		add(e, designtest.ImportID, status, "")
		svc := e.Service(100)
		d, err := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
		if err != nil || d.Status != want || len(d.Screens) != 0 || d.PendingImport != nil {
			t.Fatalf("%s: %+v err %v", status, d, err)
		}
		if _, err := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{}); !errors.Is(err, designapi.ErrNotReady) {
			t.Fatalf("%s screens: %v", status, err)
		}
		if _, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(1)); !errors.Is(err, designapi.ErrNotReady) {
			t.Fatalf("%s preview: %v", status, err)
		}
	}

	e := newEnv()
	add(e, designtest.ImportID, imports.StatusFailed, "FIGMA_PERMISSION_DENIED")
	d, _ := e.Service(100).Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if d.Status != "failed" || d.Error.Code != "FIGMA_PERMISSION_DENIED" || !strings.Contains(d.Error.Message, "permission") {
		t.Fatalf("failed = %+v", d)
	}
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw), "Layr could not") == false && strings.Contains(string(raw), "figma_alpha") {
		t.Fatal("provider detail leaked")
	}

	e = newEnv()
	if _, err := e.Service(100).Design(ctx, designtest.OwnerA, designtest.ProjectA); !errors.Is(err, designapi.ErrDesignNotFound) {
		t.Fatalf("no imports: %v", err)
	}
}

func TestAFailedRefreshNeverHidesTheCurrentDesign(t *testing.T) {
	e := designtest.New(t)
	e.Imports.Add(imports.Import{ID: "eeeeeeee-0000-4000-8000-000000000003", ProjectID: designtest.ProjectA, Status: imports.StatusFailed, ErrorCode: "FIGMA_RATE_LIMITED"})
	svc := e.Service(100)

	d, _ := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)

	if d.Status != "ready" || d.ImportID != designtest.ImportID || len(d.Screens) != 7 {
		t.Fatalf("current design lost: %+v", d)
	}
	if d.PendingImport == nil || d.PendingImport.Status != "failed" || d.PendingImport.Error.Code != "FIGMA_RATE_LIMITED" || d.PendingImport.ImportID != "eeeeeeee-0000-4000-8000-000000000003" {
		t.Fatalf("pending = %+v", d.PendingImport)
	}
	if _, err := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{}); err != nil {
		t.Fatalf("screens must keep working: %v", err)
	}

	e.Imports.Add(imports.Import{ID: "ffffffff-0000-4000-8000-000000000004", ProjectID: designtest.ProjectA, Status: imports.StatusProcessing})
	d, _ = svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if d.Status != "ready" || d.PendingImport.Status != "importing" {
		t.Fatalf("pending = %+v", d.PendingImport)
	}
}

func TestExpiredWorkspaceIsADeliberateState(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	if err := e.Manager.Cleanup(designtest.ImportID); err != nil {
		t.Fatal(err)
	}

	d, err := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil || d.Status != "expired" || d.Source.FileName != "SaaS Dashboard" || len(d.Screens) != 0 {
		t.Fatalf("summary = %+v err %v", d, err)
	}
	for name, err := range map[string]error{
		"screens": second(svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{})),
		"screen":  second(svc.Screen(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(1))),
		"preview": second(svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(1))),
	} {
		if !errors.Is(err, designapi.ErrExpired) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPreviewStreamsTheStoredImageWithItsChecksum(t *testing.T) {
	e := designtest.New(t)

	p, err := e.Service(100).Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(3))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	body, _ := io.ReadAll(p.File)
	if !bytes.Equal(body, e.PNG) || p.Size != int64(len(e.PNG)) || p.MediaType != "image/png" || p.ETag != `"`+fmt.Sprintf("%064x", 3)+`"` || !strings.HasPrefix(p.DesignVersion, "dv_") {
		t.Fatalf("preview = %+v", p)
	}
}

func TestPreviewFailuresAreDeliberate(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	dir := filepath.Join(e.Dir.Path(), workspace.Reference)

	_ = os.Remove(filepath.Join(dir, "screen-1.png"))
	if _, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(1)); !errors.Is(err, designapi.ErrPreviewUnavailable) {
		t.Errorf("missing file: %v", err)
	}
	e.WritePreview("screen-2.png", []byte("<html>not a png</html>"))
	if _, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(2)); !errors.Is(err, designapi.ErrPreviewUnavailable) {
		t.Errorf("non-png content: %v", err)
	}

	secret := filepath.Join(filepath.Dir(e.Root), "secret.txt")
	_ = os.WriteFile(secret, []byte("top secret"), 0o600)
	_ = os.Remove(filepath.Join(dir, "screen-3.png"))
	if err := os.Symlink(secret, filepath.Join(dir, "screen-3.png")); err == nil {
		if _, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(3)); !errors.Is(err, designapi.ErrPreviewUnavailable) {
			t.Errorf("symlinked preview: %v", err)
		}
	}
	if _, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, "screen_00000000000000ff"); !errors.Is(err, designapi.ErrScreenNotFound) {
		t.Errorf("unknown screen: %v", err)
	}
}

func TestStoredPathsCanNeverSelectAnotherFile(t *testing.T) {
	for _, path := range []string{
		"raw/target-node.json", "assets/manifest.json", "design/design-ir.json", "reference/../raw/target-node.json", "reference/sub/x.png",
		"/etc/passwd", "reference/UPPER.png", "reference/x.jpg", "reference/.hidden.png", "reference/x.png/../../y.png", "", "reference/",
		"reference/" + strings.Repeat("a", 80) + ".png", "reference\\x.png", "https://cdn.example/x.png",
	} {
		e := designtest.New(t)
		ir := designtest.BuildIR(7, true)
		ir.Screens[0].Reference.Path = path
		// Written directly so the file holds the hostile path even where the validator would refuse it.
		raw, err := json.Marshal(ir)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Dir.WriteBytes(workspace.Design, "design-ir.json", raw); err != nil {
			t.Fatal(err)
		}
		svc := e.Service(100)

		p, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(1))

		if err == nil {
			_ = p.Close()
			t.Errorf("%q was served", path)
			continue
		}
		if !errors.Is(err, designapi.ErrPreviewUnavailable) && !errors.Is(err, designapi.ErrInvalidDesign) {
			t.Errorf("%q: unexpected error %v", path, err)
		}
	}
}

func TestCorruptDesignFilesAreReportedNotCrashed(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	for name, content := range map[string]string{
		"garbage": "not json at all", "truncated": `{"schema_version":1,"screens":[`, "wrong version": `{"schema_version":9}`,
		"empty": "", "unknown field": `{"schema_version":1,"surprise":true}`, "null": "null",
	} {
		e.Imports.Rows = e.Imports.Rows[:1]
		if err := e.Dir.WriteBytes(workspace.Design, "design-ir.json", []byte(content)); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA); !errors.Is(err, designapi.ErrInvalidDesign) {
			t.Errorf("%s: %v", name, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestChangedDesignFilesAreReloaded(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	first, _ := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)

	time.Sleep(5 * time.Millisecond)
	e.WriteIR(designtest.BuildIR(2, false))
	second, err := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)

	if err != nil || len(second.Screens) != 2 || second.DesignVersion == first.DesignVersion {
		t.Fatalf("stale design served: %+v err %v", second, err)
	}
}

func TestConcurrentRequestsShareOneParseSafely(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
			if err != nil || len(d.Screens) != 7 {
				t.Errorf("design: %v", err)
				return
			}
			if _, err := svc.Screen(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(i%7+1)); err != nil {
				t.Errorf("screen: %v", err)
			}
			if p, err := svc.Preview(ctx, designtest.OwnerA, designtest.ProjectA, designtest.ScreenID(i%7+1)); err != nil {
				t.Errorf("preview: %v", err)
			} else {
				_ = p.Close()
			}
			page, _ := svc.Screens(ctx, designtest.OwnerA, designtest.ProjectA, designapi.Query{Limit: 3, Offset: i % 3})
			page.Screens[0].Name = "mutated by a caller"
		}(i)
	}
	wg.Wait()

	d, _ := svc.Design(ctx, designtest.OwnerA, designtest.ProjectA)
	if d.Screens[0].Name != "Landing" {
		t.Fatal("a caller changed shared state")
	}
}

func TestPinReturnsOnlyTheExactCurrentVersion(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	d, err := svc.Design(context.Background(), designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := svc.Pin(context.Background(), designtest.OwnerA, designtest.ProjectA, d.DesignVersion)
	if err != nil || pin.Version != d.DesignVersion || pin.ImportID != designtest.ImportID || len(pin.Design.Screens) != 7 {
		t.Fatalf("pin: %+v %v", pin, err)
	}
	if _, err := svc.Pin(context.Background(), designtest.OwnerA, designtest.ProjectA, "dv_0000000000000000"); !errors.Is(err, designapi.ErrVersionUnavailable) {
		t.Fatalf("stale version: %v", err)
	}
	if _, err := svc.Pin(context.Background(), designtest.OwnerB, designtest.ProjectA, d.DesignVersion); !errors.Is(err, designapi.ErrProjectNotFound) {
		t.Fatalf("stranger: %v", err)
	}
	if err := e.Manager.Cleanup(designtest.ImportID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Pin(context.Background(), designtest.OwnerA, designtest.ProjectA, d.DesignVersion); !errors.Is(err, designapi.ErrVersionUnavailable) {
		t.Fatalf("expired design: %v", err)
	}
}
