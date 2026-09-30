package designapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const irFile = "design-ir.json"

// index is a parsed design plus lookups. It is shared between requests and must never be modified.
type index struct {
	ir         *designir.DesignIR
	version    string
	position   map[string]int
	flowOf     map[string]string
	flowPos    map[string]int
	warnings   map[string][]designir.Warning
	components map[string]designir.Component
}

func buildIndex(ir *designir.DesignIR, importID, irSHA string) *index {
	sum := sha256.Sum256([]byte(importID + "\x00" + irSHA))
	ix := &index{
		ir: ir, version: "dv_" + hex.EncodeToString(sum[:])[:16], position: map[string]int{}, flowOf: map[string]string{},
		flowPos: map[string]int{}, warnings: map[string][]designir.Warning{}, components: map[string]designir.Component{},
	}
	for i, s := range ir.Screens {
		ix.position[s.ID] = i
	}
	for i, f := range ir.Sections {
		ix.flowPos[f.ID] = i
		for _, id := range f.ScreenIDs {
			ix.flowOf[id] = f.ID
		}
	}
	for _, w := range ir.Warnings {
		ix.warnings[w.ScreenID] = append(ix.warnings[w.ScreenID], w)
	}
	for _, c := range ir.Components {
		ix.components[c.ID] = c
	}
	return ix
}

type cached struct {
	ix      *index
	size    int64
	modTime time.Time
	used    uint64
}

type call struct {
	done chan struct{}
	ix   *index
	err  error
}

// loader parses each import's design once and shares the result between concurrent requests.
type loader struct {
	ws       Workspaces
	lim      designir.Limits
	maxBytes int64
	maxItems int

	mu       sync.Mutex
	entries  map[string]*cached
	inflight map[string]*call
	tick     uint64
}

func newLoader(ws Workspaces, lim designir.Limits, maxBytes int64, maxItems int) *loader {
	if maxItems < 1 {
		maxItems = 4
	}
	return &loader{ws: ws, lim: lim, maxBytes: maxBytes, maxItems: maxItems, entries: map[string]*cached{}, inflight: map[string]*call{}}
}

// load returns the design of an import, ErrExpired when its temporary workspace is gone.
func (l *loader) load(importID string) (*index, error) {
	dir, err := l.ws.Existing(importID)
	if err != nil {
		return nil, missing(err)
	}
	f, info, err := dir.OpenRegular(workspace.Design, irFile)
	if err != nil {
		return nil, missing(err)
	}
	f.Close()

	l.mu.Lock()
	if e, ok := l.entries[importID]; ok && e.size == info.Size() && e.modTime.Equal(info.ModTime()) {
		l.tick++
		e.used = l.tick
		l.mu.Unlock()
		return e.ix, nil
	}
	if c, ok := l.inflight[importID]; ok {
		l.mu.Unlock()
		<-c.done
		return c.ix, c.err
	}
	c := &call{done: make(chan struct{})}
	l.inflight[importID] = c
	l.mu.Unlock()

	c.ix, c.err = l.parse(dir, importID)

	l.mu.Lock()
	delete(l.inflight, importID)
	if c.err == nil {
		l.store(importID, &cached{ix: c.ix, size: info.Size(), modTime: info.ModTime()})
	}
	l.mu.Unlock()
	close(c.done)
	return c.ix, c.err
}

func missing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return ErrExpired
	}
	return ErrInvalidDesign
}

func (l *loader) parse(dir *workspace.Dir, importID string) (*index, error) {
	f, info, err := dir.OpenRegular(workspace.Design, irFile)
	if err != nil {
		return nil, missing(err)
	}
	defer f.Close()
	if info.Size() > l.maxBytes {
		return nil, ErrInvalidDesign
	}
	data, err := io.ReadAll(io.LimitReader(f, l.maxBytes+1))
	if err != nil || int64(len(data)) > l.maxBytes {
		return nil, ErrInvalidDesign
	}
	ir, err := designir.Unmarshal(data, l.lim)
	if err != nil {
		return nil, ErrInvalidDesign
	}
	sum := sha256.Sum256(data)
	return buildIndex(ir, importID, hex.EncodeToString(sum[:])), nil
}

// store keeps the entry and evicts the least recently used ones beyond the limit.
func (l *loader) store(key string, e *cached) {
	l.tick++
	e.used = l.tick
	l.entries[key] = e
	for len(l.entries) > l.maxItems {
		oldest, oldestKey := ^uint64(0), ""
		for k, v := range l.entries {
			if v.used < oldest {
				oldest, oldestKey = v.used, k
			}
		}
		delete(l.entries, oldestKey)
	}
}
