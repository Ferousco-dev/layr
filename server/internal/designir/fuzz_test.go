package designir

import (
	"bytes"
	"testing"
)

func FuzzUnmarshalNeverPanicsAndRoundTrips(f *testing.F) {
	good, _ := Marshal(valid(), DefaultLimits)
	f.Add(good)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema_version":1}`))
	f.Add([]byte(`null`))
	f.Add(bytes.Repeat([]byte(`{"a":`), 200))
	f.Fuzz(func(t *testing.T, data []byte) {
		ir, err := Unmarshal(data, DefaultLimits)
		if err != nil {
			return
		}
		again, err := Marshal(ir, DefaultLimits)
		if err != nil {
			t.Fatalf("a design that unmarshals must marshal: %v", err)
		}
		second, err := Unmarshal(again, DefaultLimits)
		if err != nil {
			t.Fatalf("re-reading marshalled output failed: %v", err)
		}
		third, _ := Marshal(second, DefaultLimits)
		if !bytes.Equal(again, third) {
			t.Fatal("marshal is not stable across a round trip")
		}
	})
}
