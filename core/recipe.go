package core

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/go-redis/redis/v7"
)

var (
	log = logrus.New()
)

type RecipeEngine struct {
	redisClient *redis.Client
	recipesDir  string
}

func NewRecipeEngine(redisClient *redis.Client, recipesDir string) *RecipeEngine {
	return &RecipeEngine{
		redisClient: redisClient,
		recipesDir:  recipesDir,
	}
}

type RecipeQuery struct {
	Name    string
	Version string
	e       *RecipeEngine
}

type Recipe struct {
	e             *RecipeEngine
	Name          string
	URL           *url.URL
	Draft         bool
	Version       string
	Script        []byte
	MoreMD        []byte
	MD            []byte
	Compat        map[string][]string
	Depends       map[string]*Recipe
	RemoteScripts []string
	SubstVars     []string
	Tags          []string
}

func (r *Recipe) URLNoScheme() string {
	return strings.TrimPrefix(r.URL.String(), fmt.Sprintf("%s://", r.URL.Scheme))
}

func (r *Recipe) Hits() int {
	s, err := r.e.redisClient.Get(fmt.Sprintf("%s_hits", r.Name)).Result()
	if err != nil {
		log.Error(err)
		return 0
	}
	hits, err := strconv.Atoi(s)
	if err != nil {
		log.Error(err)
		return 0
	}
	return hits

}

func (r *Recipe) DependenciesAndRemoteScriptsCount() int {
	return len(r.Depends) + len(r.RemoteScripts)
}

func (r *Recipe) LoadMetaString(s string) error {
	s = strings.TrimPrefix(s, "# nopm:")
	args := strings.Split(s, " ")
	if len(args) < 2 {
		return fmt.Errorf("Not enougth parameters meta %+v", args)
	}
	meta := args[0]
	switch meta {
	case "compat":
		for _, compat := range args[1:] {
			compatL := strings.Split(compat, "@")
			if len(compatL) != 2 {
				return fmt.Errorf("Unable to read compat %s", compat)
			}
			os := compatL[0]
			arch := compatL[1]

			r.Compat[os] = append(r.Compat[os], arch)
		}
	case "subst":
		for _, v := range args[1:] {
			r.SubstVars = append(r.SubstVars, v)
		}
	case "url":
		u, err := url.Parse(args[1])
		if err != nil {
			return fmt.Errorf("Unable to parse URL: %+v", err)
		}
		r.URL = u
	case "remote_script":
		u, err := url.Parse(args[1])
		if err != nil {
			return fmt.Errorf("Unable to parse URL: %+v", err)
		}
		r.RemoteScripts = append(r.RemoteScripts, u.String())
	case "draft":
		draft, err := strconv.ParseBool(args[1])
		if err != nil {
			return fmt.Errorf("Unable to parse bool: %+v", err)
		}
		r.Draft = draft
	case "depends":
		q := r.e.ParseRecipeRawName(args[1])
		d, err := q.Get()
		if err != nil {
			return err
		}
		r.Depends[d.Name] = d
	case "tags":
		for _, tag := range args[1:] {
			r.Tags = append(r.Tags, tag)
		}
	default:
		return fmt.Errorf("Unknown meta %s", meta)
	}
	return nil
}

func (q *RecipeQuery) Get() (*Recipe, error) {
	r := &Recipe{
		Name:    q.Name,
		Version: q.Version,
		Depends: make(map[string]*Recipe),
		e:       q.e,
	}
	// TODO need security here
	data, err := ioutil.ReadFile(fmt.Sprintf("%s/%s.sh", q.e.recipesDir, r.Name))
	if err != nil {
		return nil, err
	}
	r.Script = data
	r.Compat = make(map[string][]string)
	r.loadMD()
	r.loadMoreMD()

	scanner := bufio.NewScanner(strings.NewReader(string(r.Script)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		s := scanner.Text()
		if strings.HasPrefix(s, "# nopm:") {
			err := r.LoadMetaString(s)
			if err != nil {
				return nil, fmt.Errorf("Unable to parse meta %s: %v", s, err)
			}

		}
	}
	return r, nil
}

func (e *RecipeEngine) All() ([]*Recipe, error) {
	recipesFilenames, err := filepath.Glob(fmt.Sprintf("%s/*.sh", e.recipesDir))
	if err != nil {
		return nil, err
	}
	var recipes []*Recipe
	for _, recipeFilename := range recipesFilenames {
		if strings.HasSuffix(recipeFilename, "_test.sh") {
			continue
		}
		recipeFilename = strings.TrimSuffix(recipeFilename, path.Ext(recipeFilename))
		recipeNameSlice := strings.Split(recipeFilename, "/")
		recipeName := recipeNameSlice[len(recipeNameSlice)-1]
		q := &RecipeQuery{
			Name: recipeName,
			e:    e,
		}
		r, err := q.Get()
		if err != nil {
			log.Errorf("Unable to get recipe %s: %+v", recipeFilename, err)
			continue
		}

		recipes = append(recipes, r)
	}
	return recipes, nil
}

func (r *Recipe) CurlCmd() string {
	return fmt.Sprintf("curl nopm.sh/%s | sh", r.Name)
}

func (r *Recipe) loadMoreMD() error {
	data, err := ioutil.ReadFile(fmt.Sprintf("%s/%s.more.md", r.e.recipesDir, r.Name))
	if err != nil {
		return err
	}
	r.MoreMD = data
	return nil
}

func (r *Recipe) loadMD() error {
	data, err := ioutil.ReadFile(fmt.Sprintf("/Users/meister/recipes/%s.md", r.Name))
	if err != nil {
		return err
	}
	r.MD = data
	return nil
}

func (r *Recipe) Render() ([]byte, error) {
	var script string
	scanner := bufio.NewScanner(strings.NewReader(string(r.Script)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		s := scanner.Text()

		for _, substVar := range r.SubstVars {
			if strings.HasPrefix(s, fmt.Sprintf("%s=\"\"", substVar)) {
				switch substVar {
				case "version":
					if !isVersion(r.Version) {
						return nil, fmt.Errorf("Unable to validate version")
					}
					s = fmt.Sprintf("%s=\"%s\"", substVar, r.Version)
				}
			}
		}

		script = script + s + "\n"
	}

	return []byte(script), nil
}

func (e *RecipeEngine) ParseRecipeRawName(input string) *RecipeQuery {
	rawName := strings.Split(input, "@")
	name := rawName[0]
	version := ""
	if len(rawName) == 2 {
		version = rawName[1]
	}
	return &RecipeQuery{
		Name:    name,
		Version: version,
		e:       e,
	}
}

func (e *RecipeEngine) Search(input string) ([]string, error) {
	recipes, err := e.All()
	if err != nil {
		return nil, err
	}
	var results []string
	for _, recipe := range recipes {
		if strings.Contains(strings.ToLower(recipe.Name), input) {
			results = append(results, recipe.Name)
		}
	}
	return results, nil

}
