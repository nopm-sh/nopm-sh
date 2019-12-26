package providers

import (
	"testing"
)

func TestHashicorpReleaseUrl(t *testing.T) {
	r, err := HashicorpReleasesGet("terraform")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.GetBuild("0.12.9", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	output := b.URL
	expected := "https://releases.hashicorp.com/terraform/0.12.9/terraform_0.12.9_linux_amd64.zip"
	if output != expected {
		t.Fatalf("want %+v, got %+v", output, expected)
	}
}

func TestHashicorpReleaseLatest(t *testing.T) {
	r, err := HashicorpReleasesGet("terraform")
	if err != nil {
		t.Fatal(err)
	}
	_, errL := r.LatestVersion()
	if errL != nil {
		t.Fatal(errL)
	}
}
