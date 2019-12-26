package providers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/hashicorp/go-version"
)

type HashicorpReleaseBuild struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

type HashicorpReleaseVersion struct {
	Name             string                   `json:"name"`
	Version          string                   `json:"version"`
	Shasums          string                   `json:"shasums"`
	ShasumsSignature string                   `json:"shasums_signature"`
	Builds           []*HashicorpReleaseBuild `json:"builds"`
}

type HashicorpRelease struct {
	Name     string                              `json:"name"`
	Versions map[string]*HashicorpReleaseVersion `json:"versions"`
}

func (r *HashicorpRelease) LatestVersion() (*HashicorpReleaseVersion, error) {
	var v1 *version.Version
	for v, _ := range r.Versions {
		v2, _ := version.NewVersion(v)
		if v1 != nil && v2.LessThan(v1) {
			continue
		}
		v1 = v2
	}
	if v1 == nil {
		return nil, fmt.Errorf("Latest version not found")
	}
	return r.Versions[v1.String()], nil
}

func (r *HashicorpRelease) GetBuild(version string, os string, arch string) (*HashicorpReleaseBuild, error) {
	if r.Versions[version] == nil {
		return nil, fmt.Errorf("Version not found")
	}
	for _, b := range r.Versions[version].Builds {
		if b.OS == os && b.Arch == arch {
			return b, nil
		}
	}
	return nil, fmt.Errorf("Release not found")
}

func HashicorpReleasesGet(name string) (*HashicorpRelease, error) {
	dataURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/index.json")
	c := http.Client{
		Timeout: time.Second * 5,
	}

	req, err := http.NewRequest(http.MethodGet, dataURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "nopm.sh")

	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	r := &HashicorpRelease{}
	errJ := json.Unmarshal(body, &r)
	if errJ != nil {
		return nil, errJ
	}
	return r, nil
}
