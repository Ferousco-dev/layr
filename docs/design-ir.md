# Design IR v1

Design IR is Layr's framework-independent description of an imported design. It is the only input
later stages (code generation, repair, export) need: they never read raw Figma JSON. The normalizer
(`server/internal/normalize`) produces it deterministically from the Figma snapshot and the Asset
Manifest, using no AI, no network, no database and no Redis. The schema and validator live in
`server/internal/designir`. The file is `design/design-ir.json` in the import workspace.

## Principles

- **Facts are preserved, never inferred.** Sizes, spacing, colors, typography, Auto Layout and text are
  copied from Figma. Where Figma does not say (for example how a screen behaves on mobile), the IR says
  nothing rather than guessing.
- **No framework concepts.** There is no `display: flex`, CSS class, Tailwind token, React component or
  Figma enum in the core vocabulary. Layout is `horizontal` / `vertical` / `none`; sizing is `fixed` / `hug` /
  `fill`.
- **Portable and safe.** No credentials, signed URLs or host paths. Assets are referenced by logical ID;
  paths inside the design are relative (`assets/...`, `reference/...`).
- **Traceable.** Every node keeps its source provider, node ID and node type.
- **Versioned.** `schema_version` is `1`; any change readers must handle bumps it.
- **Deterministic.** The same snapshot and manifest give the same bytes. IDs are hashes of file key and
  source ID, ordering follows the design, and derived lists are sorted.

## A design is a project of screens

```text
DesignIR
├── source        provider, file key, file name, version, import ID, node IDs
├── screens[]     one per page-sized frame; each has an ID, a source node ID and a full node tree
├── sections[]    groups of screens the designer grouped (a flow)
├── components[]  reusable designs shared by instances, across screens
├── component_sets[], styles[]
├── assets[]      stored files used by the screens (logical IDs, relative paths, checksums)
├── tokens        recurring exact values with usage counts
├── warnings[]    approximations and unsupported features
└── stats
```

A Figma frame is **not** a whole project. An import can hold many screens, and a Figma *section* selected
as a target contributes each frame inside it as a screen (the section becomes a `sections[]` entry).
Components and assets are shared: each `components[]` entry lists the screens that use it and, when the
master is inside the imported screens, where the master is (`definition_screen_id`, `definition_node_id`).
A screen lists the components and assets it needs (`component_ids`, `asset_ids`).

`DesignIR.Select(screenIDs)` returns a complete, independent design for a chosen subset (one screen, some
screens, a section, or all) by recomputing components, assets, styles, tokens and statistics from the
selected screens. It needs no Figma data and no re-import, so a later generation planner can build the
shared foundation first and then treat each screen as a separate job. Nothing in the IR sends the whole
application to a model at once.

## Nodes

`Node` fields (all optional except `id`, `source`, `name`, `type`, `visible`, `geometry`, `children`):

| Field | Meaning |
| --- | --- |
| `type` | `document canvas section frame group text image vector shape component component_set instance unknown` |
| `shape` | `rectangle ellipse line polygon star` for `shape` nodes |
| `geometry` | `width`, `height`, parent-relative `x`, `y`, `rotation` (degrees, counter-clockwise as Figma shows), and `absolute` source bounds |
| `sizing` | `horizontal` and `vertical` axes with `mode` fixed/hug/fill and optional `min`/`max`; absent means fixed on both |
| `position` | `mode` flow or absolute, `constraints` (left, right, center, left_right, scale, top, bottom, top_bottom), `align: stretch`, `grow` |
| `layout` | container arrangement: `mode` none/horizontal/vertical/grid, `justify`, `align`, `gap`, `cross_gap`, `padding`, `wrap` |
| `appearance` | `opacity`, `blend_mode`, `fills`, `stroke`, `radius`, `effects` |
| `text` | exact `characters`, base `style`, and `runs` for mixed styling |
| `asset` | a vector drawn by an exported SVG asset |
| `component`, `instance` | master / set membership, and instance link with properties and overrides |
| `style_refs` | links to named Figma styles by role |
| `clip_children`, `mask` | clipping and mask flags |
| `collapsed_children` | descendants folded into an exported vector |

**Unknown types** become `unknown` with `source.type` preserved, their children kept and a warning.
**Hidden layers** are kept with `visible: false`. **Child order** is the source order, which is paint order;
no z-index numbers are invented.

## Geometry

Figma gives absolute bounding boxes. Each node's `x`, `y` are relative to its parent's top-left corner and
the untouched source box stays in `geometry.absolute` for visual checks. For a rotated layer Figma only
supplies the bounds of the rotated shape, so the unrotated width and height are solved from those bounds
(exact for any angle except near 45°, where no unique solution exists and a `ROTATED_BOUNDS_APPROXIMATE`
warning is recorded), and `x`, `y` follow the layer's center.

**Float noise.** Numbers within 0.001 of a whole number become that whole number (`63.999996` → `64`);
other values keep four decimals (`12.5`, `0.3333`). Colors are 0-255 integers (rounded to nearest) with alpha
0-1 to four decimals.

## Layout, sizing and constraints

- Auto Layout maps to `layout` (mode, `justify` start/center/end/space_between, `align` start/center/end/baseline,
  padding per side, wrap). With `space_between` the gap is automatic, so **no numeric gap is emitted**.
- `sizing` uses Figma's explicit fixed/hug/fill when present. For older files it is derived from the
  container's sizing mode, and from a child's stretch and grow.
- A child inside Auto Layout is `flow`, unless Figma marks it absolute; everywhere else children are
  `absolute` with their constraints, which are evidence for responsive behavior later.
- Frames without Auto Layout have `layout.mode: none`. Grid layout is recorded as `grid` with a warning
  (row and column details are not captured yet).
- Responsiveness is not inferred: no breakpoints are invented, and several frames of one product simply
  appear as separate screens.

## Text

`characters` is stored exactly as written (no trimming, case change or translation). The base `style`
holds font family, PostScript name, numeric weight (only when Figma gives one), size, italic, `line_height`
(`auto`, `pixels` or `percent`, keeping Figma's unit), `letter_spacing_px`, alignment, case, decoration,
auto-resize, paragraph settings and hyperlink. Mixed styling becomes `runs` over code points
(`[start, end)`), each with the fields that differ. Figma counts UTF-16 units; they are converted, so an
emoji is one position. Mismatched lengths produce `MIXED_STYLE_PARTIAL` and no runs. Fonts are not
downloaded; only names are recorded.

## Appearance

- **Paints:** `solid`, `linear_gradient`, `radial_gradient`, `angular_gradient`, `diamond_gradient`,
  `image`, plus `video`, `pattern` and `unknown` (kept with `source_type` and an `UNSUPPORTED_PAINT` warning).
  Gradients keep every stop and the handle positions. Paint opacity and node opacity stay separate.
- **Image paints** reference an asset: `asset_id`, `scale_mode` (fill, fit, tile, stretch), `transform`,
  `rotation`, `scaling_factor`, `filters`. A missing asset sets `missing: true` and warns; the visual is
  never silently dropped.
- **Stroke:** paints, weight, per-side weights, alignment, dashes, cap, join, miter angle.
- **Radius:** four corners; omitted when there is no rounding.
- **Effects:** ordered list of `drop_shadow`, `inner_shadow`, `layer_blur`, `background_blur` with offset,
  radius, spread, color, visibility; unknown types are kept and warned.
- **Blend modes:** lower-case canonical names; defaults are omitted; unknown ones are kept as written with a warning.

## Assets

The normalizer reads the Asset Manifest (`assets/manifest.json`). Image fills resolve `imageRef` to the
asset ID; a vector that was exported as SVG becomes a `vector` node with `asset.asset_id`, and the shapes
inside it are folded away (`collapsed_children`). No path data is embedded in the IR. `assets[]` lists only
assets some screen uses, with the screens that use them. Each screen also carries the `reference` render.

## Components

A component master keeps `component.component_id`; an instance keeps `instance.component_id`, the source
component ID, its component properties and its overrides (which nodes changed which fields). Instances also
keep their rendered children so a screen stays complete on its own. If a master is outside the imported
screens the component entry has no definition location. Repetition that Figma does not state as component
instances is not detected here.

## Tokens

`tokens` lists recurring exact values with usage counts: colors (solid fills, strokes, text), typography
(family, weight, size, line height, letter spacing), spacing (gaps and paddings), radii and shadows.
They describe what the design already uses; nodes keep their own values. Named Figma styles appear in
`styles[]` and are linked from nodes through `style_refs`.

## Warnings and errors

Warnings are `{code, source_node_id, screen_id, message}`, sorted and de-duplicated, with short safe
messages that contain no design text. Codes: `UNKNOWN_NODE_TYPE`, `UNSUPPORTED_PAINT`, `UNSUPPORTED_EFFECT`,
`UNSUPPORTED_MASK`, `UNSUPPORTED_LAYOUT_GRID`, `UNKNOWN_BLEND_MODE`, `MISSING_ASSET`, `MISSING_BOUNDS`,
`MIXED_STYLE_PARTIAL`, `ROTATED_BOUNDS_APPROXIMATE`, and `ASSET_*` for warnings the asset pipeline raised.

Fatal errors: `DESIGN_IR_INVALID_INPUT`, `DESIGN_IR_ROOT_MISSING`, `DESIGN_IR_LIMIT_EXCEEDED`,
`DESIGN_IR_VALIDATION_FAILED`, `DESIGN_IR_SERIALIZATION_FAILED`. Rule of thumb: anything that leaves a
usable design (one unsupported effect, one missing decorative asset) is a warning; no screens, an invalid
tree or exceeded limits is an error.

## Validation and limits

`designir.Validate` runs before every write and on every read: schema version, source, at least one screen,
unique IDs, screen/root consistency, valid node types, no children on leaf types, text present exactly on
text nodes, valid text runs, all asset/component/style/section references resolve, colors and opacities in
range, no NaN or infinity anywhere, and safe relative asset paths (no absolute paths, URLs or `..`). Traversal
is bounded: 200,000 nodes, 256 levels and 500 screens by default (`MAX_IR_NODES`, `MAX_SCREENS`), and the
serialized file is capped by `MAX_IR_BYTES` (64 MiB). The file is compact JSON (no indentation), about 380 bytes
per node in the 300-screen stress fixture, so the byte cap and the node cap are consistent. The JSON reader is strict: unknown fields and trailing
data are rejected.

## Storage

`design/design-ir.json` is written atomically (temporary file, sync, rename) in the import workspace, next to
`raw/`, `reference/` and `assets/`. PostgreSQL keeps only counts (screens, nodes, warnings). It is temporary
like the rest of the workspace and can be rebuilt by importing again.

## Example

A hero frame with Auto Layout, a heading, a component button and an exported illustration becomes (abridged):

```json
{
  "schema_version": 1,
  "source": {"provider": "figma", "file_key": "FILEKEY123456", "node_ids": ["12:34"]},
  "screens": [{
    "id": "screen_2fd6f8348e564969", "source_node_id": "12:34", "name": "Hero", "width": 1440, "height": 720,
    "component_ids": ["comp_b8578de3f2a242e7"], "asset_ids": ["asset_0123456789abcdef"],
    "root": {
      "id": "n_...", "source": {"provider": "figma", "node_id": "12:34", "type": "FRAME"}, "name": "Hero", "type": "frame", "visible": true,
      "geometry": {"width": 1440, "height": 720, "x": 0, "y": 0, "absolute": {"x": 100, "y": 200, "width": 1440, "height": 720}},
      "sizing": {"horizontal": {"mode": "fixed"}, "vertical": {"mode": "hug"}},
      "layout": {"mode": "horizontal", "justify": "space_between", "align": "center",
                 "padding": {"top": 64, "right": 120, "bottom": 64, "left": 120}},
      "clip_children": true,
      "children": [
        {"name": "Text Group", "type": "frame", "position": {"mode": "flow"},
         "layout": {"mode": "vertical", "gap": 24, "justify": "start", "align": "start"},
         "children": [
           {"name": "Heading", "type": "text",
            "text": {"characters": "Build faster with Layr\nToday",
                     "style": {"font_family": "Inter", "font_weight": 600, "font_size": 64,
                               "line_height": {"mode": "pixels", "value": 72}},
                     "runs": [{"start": 5, "end": 22, "style": {"font_weight": 300}}]}},
           {"name": "CTA", "type": "instance",
            "instance": {"component_id": "comp_b8578de3f2a242e7", "component_source_id": "5:100"}}]},
        {"name": "Illustration", "type": "vector", "position": {"mode": "flow"},
         "asset": {"asset_id": "asset_0123456789abcdef", "role": "export"}, "collapsed_children": 2}]}
  }],
  "components": [{"id": "comp_b8578de3f2a242e7", "name": "Button/Primary", "instance_count": 1}],
  "assets": [{"id": "asset_0123456789abcdef", "kind": "svg", "path": "assets/illustration-0123ab.svg"}]
}
```

The full expected output for this fixture is the golden file
`server/internal/normalize/testdata/hero.golden.json`; changing the schema changes that file, so schema
changes are always deliberate (`go test ./internal/normalize -update` regenerates it).
