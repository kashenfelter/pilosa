// Copyright 2017 Pilosa Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pilosa_test

import (
	"io/ioutil"
	"reflect"
	"strings"
	"testing"

	"github.com/pilosa/pilosa"
	"github.com/pilosa/pilosa/internal"
	"github.com/pilosa/pilosa/test"
)

// Ensure index can open and retrieve a frame.
func TestIndex_CreateFrameIfNotExists(t *testing.T) {
	index := test.MustOpenIndex()
	defer index.Close()

	// Create frame.
	f, err := index.CreateFrameIfNotExists("f", pilosa.FrameOptions{})
	if err != nil {
		t.Fatal(err)
	} else if f == nil {
		t.Fatal("expected frame")
	}

	// Retrieve existing frame.
	other, err := index.CreateFrameIfNotExists("f", pilosa.FrameOptions{})
	if err != nil {
		t.Fatal(err)
	} else if f.Frame != other.Frame {
		t.Fatal("frame mismatch")
	}

	if f.Frame != index.Frame("f") {
		t.Fatal("frame mismatch")
	}
}

func TestIndex_CreateFrame(t *testing.T) {
	// Ensure time quantum can be set appropriately on a new frame.
	t.Run("TimeQuantum", func(t *testing.T) {
		t.Run("Explicit", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			// Create frame with explicit quantum.
			f, err := index.CreateFrame("f", pilosa.FrameOptions{TimeQuantum: pilosa.TimeQuantum("YMDH")})
			if err != nil {
				t.Fatal(err)
			} else if q := f.TimeQuantum(); q != pilosa.TimeQuantum("YMDH") {
				t.Fatalf("unexpected frame time quantum: %s", q)
			}
		})
	})

	// Ensure frame can include range columns.
	t.Run("RangeEnabled", func(t *testing.T) {
		t.Run("OK", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			// Create frame with schema and verify it exists.
			if f, err := index.CreateFrame("f", pilosa.FrameOptions{
				RangeEnabled: false,
				Fields: []*pilosa.Field{
					{Name: "field0", Type: pilosa.FieldTypeInt, Min: 10, Max: 20},
					{Name: "field1", Type: pilosa.FieldTypeInt, Min: 11, Max: 21},
				},
			}); err != nil {
				t.Fatal(err)
			} else if !reflect.DeepEqual(f.Fields(), []*pilosa.Field{
				{Name: "field0", Type: pilosa.FieldTypeInt, Min: 10, Max: 20},
				{Name: "field1", Type: pilosa.FieldTypeInt, Min: 11, Max: 21},
			}) {
				t.Fatalf("unexpected fields: %#v", f.Fields())
			}

			// Reopen the index & verify the fields are loaded.
			if err := index.Reopen(); err != nil {
				t.Fatal(err)
			} else if f := index.Frame("f"); !reflect.DeepEqual(f.Fields(), []*pilosa.Field{
				{Name: "field0", Type: pilosa.FieldTypeInt, Min: 10, Max: 20},
				{Name: "field1", Type: pilosa.FieldTypeInt, Min: 11, Max: 21},
			}) {
				t.Fatalf("unexpected fields after reopen: %#v", f.Fields())
			}
		})

		t.Run("ErrInverseRangeAllowed", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			frame, err := index.CreateFrame("f", pilosa.FrameOptions{
				RangeEnabled:   true,
				InverseEnabled: true,
				Fields: []*pilosa.Field{
					&pilosa.Field{
						Name: "myfield",
						Type: pilosa.FieldTypeInt,
						Min:  -20,
						Max:  100,
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			ch, err := frame.SetBit(pilosa.ViewStandard, 1, 2, nil)
			if !ch || err != nil {
				t.Fatal(ch, err)
			}
			ch, err = frame.SetBit(pilosa.ViewInverse, 1, 2, nil)
			if !ch || err != nil {
				t.Fatal(ch, err)
			}
			ch, err = frame.SetFieldValue(1, "myfield", 87)
			if !ch || err != nil {
				t.Fatal(ch, err)
			}
			views := frame.Views()
			if len(views) != 3 {
				var names string
				for _, v := range views {
					names = names + v.Name() + " "
				}
				t.Fatalf("Unexpected views: %s", names)
			}

		})

		t.Run("ErrRangeCacheAllowed", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			if _, err := index.CreateFrame("f", pilosa.FrameOptions{
				CacheType: pilosa.CacheTypeRanked,
			}); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("RangeEnabledWithCacheTypeNone", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()
			if _, err := index.CreateFrame("f", pilosa.FrameOptions{
				CacheType: pilosa.CacheTypeNone,
				CacheSize: uint32(5),
			}); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("ErrFrameFieldsAllowed", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			if _, err := index.CreateFrame("f", pilosa.FrameOptions{
				Fields: []*pilosa.Field{
					{Name: "field0", Type: pilosa.FieldTypeInt},
				},
			}); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("ErrFieldNameRequired", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			if _, err := index.CreateFrame("f", pilosa.FrameOptions{
				Fields: []*pilosa.Field{
					{Name: "", Type: pilosa.FieldTypeInt},
				},
			}); err != pilosa.ErrFieldNameRequired {
				t.Fatal(err)
			}
		})

		t.Run("ErrInvalidFieldType", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			if _, err := index.CreateFrame("f", pilosa.FrameOptions{
				Fields: []*pilosa.Field{
					{Name: "field0", Type: "bad_type"},
				},
			}); err != pilosa.ErrInvalidFieldType {
				t.Fatal(err)
			}
		})

		t.Run("ErrInvalidFieldRange", func(t *testing.T) {
			index := test.MustOpenIndex()
			defer index.Close()

			if _, err := index.CreateFrame("f", pilosa.FrameOptions{
				RangeEnabled: true, // make sure we can still create frames with RangeEnabled: true after deprecation
				Fields: []*pilosa.Field{
					{Name: "field0", Type: pilosa.FieldTypeInt, Min: 100, Max: 50},
				},
			}); err != pilosa.ErrInvalidFieldRange {
				t.Fatal(err)
			}
		})
	})
}

// Ensure index can delete a frame.
func TestIndex_DeleteFrame(t *testing.T) {
	index := test.MustOpenIndex()
	defer index.Close()

	// Create frame.
	if _, err := index.CreateFrameIfNotExists("f", pilosa.FrameOptions{}); err != nil {
		t.Fatal(err)
	}

	// Delete frame & verify it's gone.
	if err := index.DeleteFrame("f"); err != nil {
		t.Fatal(err)
	} else if index.Frame("f") != nil {
		t.Fatal("expected nil frame")
	}

	// Delete again to make sure it doesn't error.
	if err := index.DeleteFrame("f"); err != nil {
		t.Fatal(err)
	}
}

// Ensure index can delete a frame.
func TestIndex_InvalidName(t *testing.T) {
	path, err := ioutil.TempDir("", "pilosa-index-")
	if err != nil {
		panic(err)
	}
	index, err := pilosa.NewIndex(path, "ABC")
	if index != nil {
		t.Fatalf("unexpected index name %v", index)
	}
}

func TestIndex_CreateInputDefinition(t *testing.T) {
	index := test.MustOpenIndex()
	defer index.Close()

	// Create Input Definition.
	frames := internal.Frame{Name: "f", Meta: &internal.FrameMeta{}}
	action := internal.InputDefinitionAction{Frame: "f", ValueDestination: "mapping", ValueMap: map[string]uint64{"Green": 1}}
	field := internal.InputDefinitionField{Name: "id", PrimaryKey: true, InputDefinitionActions: []*internal.InputDefinitionAction{&action}}
	def := internal.InputDefinition{Name: "test", Frames: []*internal.Frame{&frames}, Fields: []*internal.InputDefinitionField{&field}}
	inputDef, err := index.CreateInputDefinition(&def)
	if err != nil {
		t.Fatal(err)
	} else if inputDef.Frames()[0].Name != frames.Name {
		t.Fatalf("unexpected input definition frames %v", inputDef.Frames())
	} else if inputDef.Fields()[0].Name != field.Name {
		t.Fatalf("unexpected input definition actions %v", inputDef.Fields())
	}
}

// Ensure create input definition handle correct error
func TestIndex_CreateExistingInputDefinition(t *testing.T) {
	index := test.MustOpenIndex()
	defer index.Close()

	//Test input definition name is required
	def := internal.InputDefinition{Name: "", Frames: []*internal.Frame{}, Fields: []*internal.InputDefinitionField{}}
	_, err := index.CreateInputDefinition(&def)
	if err != pilosa.ErrInputDefinitionNameRequired {
		t.Fatal(err)
	}

	// Create Input Definition.
	frames := internal.Frame{Name: "f", Meta: &internal.FrameMeta{}}
	action := internal.InputDefinitionAction{Frame: "f", ValueDestination: "mapping", ValueMap: map[string]uint64{"Green": 1}}
	fields := internal.InputDefinitionField{Name: "id", PrimaryKey: true, InputDefinitionActions: []*internal.InputDefinitionAction{&action}}
	def = internal.InputDefinition{Name: "test", Frames: []*internal.Frame{&frames}, Fields: []*internal.InputDefinitionField{&fields}}
	_, err = index.CreateInputDefinition(&def)
	if err != nil {
		t.Fatal(err)
	}
	_, err = index.CreateInputDefinition(&def)
	if err != pilosa.ErrInputDefinitionExists {
		t.Fatal(err)
	}
}

// Ensure to delete existing input definition.
func TestIndex_DeleteInputDefinition(t *testing.T) {
	index := test.MustOpenIndex()
	defer index.Close()

	// Create Input Definition.
	frames := internal.Frame{Name: "f", Meta: &internal.FrameMeta{}}
	action := internal.InputDefinitionAction{Frame: "f", ValueDestination: "mapping", ValueMap: map[string]uint64{"Green": 1}}
	fields := internal.InputDefinitionField{Name: "id", PrimaryKey: true, InputDefinitionActions: []*internal.InputDefinitionAction{&action}}
	def := internal.InputDefinition{Name: "test", Frames: []*internal.Frame{&frames}, Fields: []*internal.InputDefinitionField{&fields}}
	_, err := index.CreateInputDefinition(&def)
	if err != nil {
		t.Fatal(err)
	}

	_, err = index.InputDefinition("test")
	if err != nil {
		t.Fatal(err)
	}

	err = index.DeleteInputDefinition("test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = index.InputDefinition("test")
	if err != pilosa.ErrInputDefinitionNotFound {
		t.Fatal(err)
	}
}

// Ensure that frame in input definition will be created when server restart
func TestIndex_CreateFrameWhenOpenInputDefinition(t *testing.T) {
	index := test.MustOpenIndex()
	defer index.Close()

	// Create Input Definition.
	frames := internal.Frame{Name: "f", Meta: &internal.FrameMeta{}}
	action := internal.InputDefinitionAction{Frame: "f", ValueDestination: "mapping", ValueMap: map[string]uint64{"Green": 1}}
	fields := internal.InputDefinitionField{Name: "id", PrimaryKey: true, InputDefinitionActions: []*internal.InputDefinitionAction{&action}}
	def := internal.InputDefinition{Name: "test", Frames: []*internal.Frame{&frames}, Fields: []*internal.InputDefinitionField{&fields}}
	input, err := index.CreateInputDefinition(&def)
	if err != nil {
		t.Fatal(err)
	}

	input.AddFrame(pilosa.InputFrame{Name: "f1"})
	index.Reopen()
	if index.Frame("f1") == nil {
		t.Fatal("Frame does not created when open index")
	}

}

func TestIndex_InputBits(t *testing.T) {
	var bits []*pilosa.Bit
	index := test.MustOpenIndex()
	defer index.Close()

	err := index.InputBits("f", bits)
	if !strings.Contains(err.Error(), "Frame not found") {
		t.Fatalf("Expected Frame not found error, actual error: %s", err)
	}

	// Create frame.
	if _, err := index.CreateFrameIfNotExists("f", pilosa.FrameOptions{TimeQuantum: pilosa.TimeQuantum("YM")}); err != nil {
		t.Fatal(err)
	}

	bits = append(bits, &pilosa.Bit{RowID: 0, ColumnID: 0})
	bits = append(bits, &pilosa.Bit{RowID: 0, ColumnID: 1})
	bits = append(bits, &pilosa.Bit{RowID: 2, ColumnID: 2, Timestamp: 1})
	bits = append(bits, nil)

	err = index.InputBits("f", bits)
	if err != nil {
		t.Fatal(err)
	}

	f := index.Frame("f")
	v := f.View(pilosa.ViewStandard)
	fragment := v.Fragment(0)

	// Verify the Bits were set
	if a := fragment.Row(0).Bits(); !reflect.DeepEqual(a, []uint64{0, 1}) {
		t.Fatalf("unexpected bits: %+v", a)
	}
}
