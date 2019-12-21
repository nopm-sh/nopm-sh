package core

import (
	"testing"
)

func TestRecipeLoadMetaString(t *testing.T) {
	recipe := &Recipe{
		Compat: make(map[string][]string),
	}
	err := recipe.LoadMetaString("# nopm:compat linux@x86_64 darwin@i386")
	if err != nil {
		t.Fatal(err)
	}
	outputI := len(recipe.Compat)
	expectedI := 2
	if outputI != expectedI {
		t.Fatalf("want %+v, got %+v", expectedI, outputI)
	}

	output := recipe.Compat["linux"][0]
	expected := "x86_64"
	if output != expected {
		t.Fatalf("want %+v, got %+v", expected, output)
	}

}

func TestRecipeSubstVars(t *testing.T) {
	recipe := &Recipe{
		Script: []byte("version=\"\"\n"),
	}
	recipe.Version = "4.0.0"
	err := recipe.LoadMetaString("# nopm:subst version")
	if err != nil {
		t.Fatal(err)
	}
	outputI := len(recipe.SubstVars)
	expectedI := 1
	if outputI != expectedI {
		t.Fatalf("want %+v, got %+v", expectedI, outputI)
	}

	output := recipe.SubstVars[0]
	expected := "version"
	if output != expected {
		t.Fatalf("want %+v, got %+v", expected, output)
	}

	outputB, err := recipe.Render()
	if err != nil {
		t.Fatal(err)
	}
	expectedB := []byte("version=\"4.0.0\"\n")
	if string(outputB) != string(expectedB) {
		t.Fatalf("want %+v, got %+v", string(expectedB), string(outputB))
	}
}
