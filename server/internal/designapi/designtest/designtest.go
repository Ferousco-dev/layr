// Package designtest builds a complete stored design (workspace, Design IR, previews, fakes) for tests.
package designtest

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/imports"
	"github.com/ferousco-dev/layr/server/internal/project"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const (
	OwnerA    = "aaaaaaaa-0000-4000-8000-000000000001"
	OwnerB    = "bbbbbbbb-0000-4000-8000-000000000002"
	ProjectA  = "aaaaaaaa-1111-4111-8111-000000000001"
	ProjectB  = "bbbbbbbb-1111-4111-8111-000000000002"
	ImportID  = "cccccccc-0000-4000-8000-000000000001"
	ImportB   = "dddddddd-0000-4000-8000-000000000002"
	Limits500 = 500
)

// Projects is an in-memory project ownership table.
type Projects struct{ Owners map[string]string }

func (p *Projects) Get(_ context.Context, owner, id string) (project.Project, error) {
	if p.Owners[id] != owner {
		return project.Project{}, project.ErrNotFound
	}
	return project.Project{ID: id, UserID: owner, Name: "p"}, nil
}

// Imports is an in-memory import table scoped by project owner.
type Imports struct {
	mu   sync.Mutex
	Rows []imports.Import
	Own  map[string]string
}

func (i *Imports) Add(imp imports.Import) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Rows = append(i.Rows, imp)
}

func (i *Imports) Latest(_ context.Context, owner, project string) (imports.Import, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Own[project] == owner {
		for k := len(i.Rows) - 1; k >= 0; k-- {
			if i.Rows[k].ProjectID == project {
				return i.Rows[k], nil
			}
		}
	}
	return imports.Import{}, imports.ErrNotFound
}

func (i *Imports) LatestCompleted(_ context.Context, owner, project string) (imports.Import, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Own[project] == owner {
		for k := len(i.Rows) - 1; k >= 0; k-- {
			if i.Rows[k].ProjectID == project && i.Rows[k].Status == imports.StatusCompleted {
				return i.Rows[k], nil
			}
		}
	}
	return imports.Import{}, imports.ErrNotFound
}

// Env is a project owned by OwnerA with a completed multi-screen import.
type Env struct {
	T        testing.TB
	Manager  *workspace.Manager
	Root     string
	Projects *Projects
	Imports  *Imports
	IR       *designir.DesignIR
	Dir      *workspace.Dir
	PNG      []byte
}

func ScreenID(n int) string  { return fmt.Sprintf("screen_%016x", n) }
func SectionID(n int) string { return fmt.Sprintf("section_%016x", n) }

func node(id, name, typ string, children ...designir.Node) designir.Node {
	return designir.Node{
		ID: "n_" + id, Source: designir.SourceRef{Provider: "figma", NodeID: id, Type: "FRAME"}, Name: name, Type: typ, Visible: true,
		Geometry: designir.Geometry{Width: 1440, Height: 900}, Children: children,
	}
}

// Fixture screens: Landing (ungrouped), Authentication (Login, Signup, Forgot Password), Dashboard (Overview, Analytics, Settings).
var Names = []string{"Landing", "Login", "Signup", "Forgot Password", "Overview", "Analytics", "Settings"}

// BuildIR returns a valid finalized design with the given number of screens (the fixture names first).
func BuildIR(count int, withFlows bool) *designir.DesignIR {
	ir := &designir.DesignIR{
		SchemaVersion: designir.SchemaVersion,
		Source:        designir.Source{Provider: "figma", FileKey: "FILEKEY123456", FileName: "SaaS Dashboard", Version: "77", ImportID: ImportID, NodeIDs: []string{}},
		Components:    []designir.Component{{ID: "comp_1", SourceID: "5:100", Name: "Button", ScreenIDs: []string{}}},
	}
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("Screen %d", i+1)
		if i < len(Names) {
			name = Names[i]
		}
		btn := node(fmt.Sprintf("b%d", i), "Button", designir.TypeInstance)
		btn.Instance = &designir.Instance{ComponentID: "comp_1", ComponentSourceID: "5:100"}
		root := node(fmt.Sprintf("s%d", i), name, designir.TypeFrame, btn)
		root.Layout = &designir.Layout{Mode: designir.LayoutVertical}
		w, h := 1440.0, 900.0
		s := designir.Screen{
			ID: ScreenID(i + 1), SourceNodeID: root.Source.NodeID, Name: name, Page: "Page 1", Width: 1440, Height: 900, Root: root,
			Reference: &designir.Reference{Path: fmt.Sprintf("reference/screen-%d.png", i+1), Width: &w, Height: &h, SHA256: fmt.Sprintf("%064x", i+1)},
		}
		ir.Screens = append(ir.Screens, s)
		ir.Source.NodeIDs = append(ir.Source.NodeIDs, s.SourceNodeID)
	}
	if withFlows && count >= 7 {
		ir.Sections = []designir.Section{
			{ID: SectionID(1), SourceNodeID: "sec-1", Name: "Authentication", ScreenIDs: []string{ScreenID(2), ScreenID(3), ScreenID(4)}},
			{ID: SectionID(2), SourceNodeID: "sec-2", Name: "Dashboard", ScreenIDs: []string{ScreenID(5), ScreenID(6), ScreenID(7)}},
		}
		for i := range ir.Screens {
			switch {
			case i >= 1 && i <= 3:
				ir.Screens[i].SectionID = SectionID(1)
			case i >= 4 && i <= 6:
				ir.Screens[i].SectionID = SectionID(2)
			}
		}
	}
	if count >= 6 {
		ir.Warnings = []designir.Warning{
			{Code: "UNSUPPORTED_PAINT", ScreenID: ScreenID(6), SourceNodeID: "s5", Message: "internal wording that must never reach the frontend /tmp/secret"},
			{Code: "SOMETHING_NEW", SourceNodeID: "s0", Message: "another internal message"},
		}
	}
	ir.Finalize()
	return ir
}

// New creates the standard environment: seven screens, two flows, previews on disk, a completed import.
func New(t testing.TB) *Env {
	t.Helper()
	return NewWith(t, BuildIR(7, true))
}

// NewWith stores the given design as project A's completed import.
func NewWith(t testing.TB, ir *designir.DesignIR) *Env {
	t.Helper()
	root := t.TempDir()
	mgr, err := workspace.NewManager(root, 64<<20, 256<<20)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := mgr.Create(ImportID)
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{
		T: t, Manager: mgr, Root: root, IR: ir, Dir: dir,
		Projects: &Projects{Owners: map[string]string{ProjectA: OwnerA, ProjectB: OwnerB}},
		Imports:  &Imports{Own: map[string]string{ProjectA: OwnerA, ProjectB: OwnerB}},
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3)))
	e.PNG = buf.Bytes()

	e.WriteIR(ir)
	for _, s := range ir.Screens {
		if s.Reference != nil {
			e.WritePreview(filepath.Base(s.Reference.Path), e.PNG)
		}
	}
	e.Imports.Add(imports.Import{
		ID: ImportID, ProjectID: ProjectA, FileKey: "FILEKEY123456", FileName: "SaaS Dashboard", Version: "77",
		Status: imports.StatusCompleted, ScreenCount: len(ir.Screens), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	return e
}

func (e *Env) WriteIR(ir *designir.DesignIR) {
	e.T.Helper()
	data, err := designir.Marshal(ir, designir.Limits{MaxNodes: 1000000, MaxDepth: 256, MaxScreens: 1000})
	if err != nil {
		e.T.Fatal(err)
	}
	if err := e.Dir.WriteBytes(workspace.Design, "design-ir.json", data); err != nil {
		e.T.Fatal(err)
	}
}

func (e *Env) WritePreview(name string, data []byte) {
	e.T.Helper()
	if err := e.Dir.WriteBytes(workspace.Reference, name, data); err != nil {
		e.T.Fatal(err)
	}
}

// WriteAsset stores an asset file in the import's workspace.
func (e *Env) WriteAsset(name string, data []byte) {
	e.T.Helper()
	if err := e.Dir.WriteBytes(workspace.Assets, name, data); err != nil {
		e.T.Fatal(err)
	}
}

// Service builds the design service over the environment.
func (e *Env) Service(maxScreens int) *designapi.Service {
	return designapi.New(e.Projects, e.Imports, e.Manager, nil, designapi.Config{
		Limits:     designir.Limits{MaxNodes: 1000000, MaxDepth: 256, MaxScreens: maxScreens},
		MaxIRBytes: 64 << 20,
	})
}
