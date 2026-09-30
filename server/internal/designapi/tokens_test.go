package designapi_test

import (
	"errors"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/designapi/designtest"
	"github.com/ferousco-dev/layr/server/internal/designir"
)

func TestTokensListColorsFontsAndNumbersInAStableOrder(t *testing.T) {
	weight := func(w float64) *float64 { return &w }
	ir := designtest.BuildIR(2, false)
	ir.Tokens = designir.Tokens{
		Colors: []designir.ColorToken{
			{ID: "c1", Value: designir.Color{R: 15, G: 23, B: 42, A: 1}, Count: 2},
			{ID: "c2", Value: designir.Color{R: 59, G: 130, B: 246, A: 1}, Count: 9},
			{ID: "c3", Value: designir.Color{R: 0, G: 0, B: 0, A: 0.25}, Count: 2},
		},
		Typography: []designir.TypographyToken{
			{ID: "t1", FontFamily: "Inter", FontWeight: weight(400), FontSize: 14, Count: 5},
			{ID: "t2", FontFamily: "Inter", FontWeight: weight(700), FontSize: 32, Count: 2},
			{ID: "t3", FontFamily: "DM Mono", FontWeight: weight(400), FontSize: 12, Count: 1},
		},
		Spacing: []designir.NumberToken{{ID: "s1", Value: 16, Count: 3}, {ID: "s2", Value: 8, Count: 3}, {ID: "s3", Value: 4, Count: 9}},
		Radii:   []designir.NumberToken{{ID: "r1", Value: 12, Count: 1}},
	}
	e := designtest.NewWith(t, ir)

	got, err := e.Service(100).Tokens(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Colors) != 3 || got.Colors[0].Hex != "#3B82F6" || got.Colors[1].Hex != "#000000" || got.Colors[1].Alpha != 0.25 || got.Colors[2].Hex != "#0F172A" {
		t.Fatalf("colors = %+v", got.Colors)
	}
	if len(got.Fonts) != 2 || got.Fonts[0].Family != "Inter" || got.Fonts[0].Count != 7 || len(got.Fonts[0].Weights) != 2 || got.Fonts[0].Weights[1] != 700 || got.Fonts[0].Sizes[0] != 14 {
		t.Fatalf("fonts = %+v", got.Fonts)
	}
	if got.Spacing[0] != 4 || got.Spacing[1] != 8 || got.Spacing[2] != 16 || len(got.Radii) != 1 {
		t.Fatalf("numbers = %v %v", got.Spacing, got.Radii)
	}
}

func TestTokensAreScopedToTheOwnerAndNeedAReadyDesign(t *testing.T) {
	e := designtest.New(t)
	svc := e.Service(100)
	if _, err := svc.Tokens(ctx, designtest.OwnerB, designtest.ProjectA); !errors.Is(err, designapi.ErrProjectNotFound) {
		t.Fatalf("a stranger: %v", err)
	}
	got, err := svc.Tokens(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil || got.Colors == nil || got.Fonts == nil || got.Spacing == nil || got.Radii == nil {
		t.Fatalf("empty tokens must still be empty lists, not null: %+v %v", got, err)
	}
}
