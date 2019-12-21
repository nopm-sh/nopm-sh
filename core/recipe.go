package core

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
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
	e       *RecipeEngine
	Name    string
	URL     *url.URL
	Draft   bool
	Version string
	Script  []byte
	MoreMD  []byte
	MD      []byte
	Compat  map[string][]string
	Depends map[string]*Recipe
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
		if strings.HasPrefix(s, "# nopm:compat ") {
			_s := strings.TrimPrefix(s, "# nopm:compat ")
			compatInput := strings.Split(_s, " ")
			if len(compatInput) != 2 {
				return nil, fmt.Errorf("Unable to parse meta: %s", s)
			}
			os := compatInput[0]
			arch := compatInput[1]
			r.Compat[os] = append(r.Compat[os], arch)
			continue
		}
		if strings.HasPrefix(s, "# nopm:url ") {
			rawURL := strings.TrimPrefix(s, "# nopm:url ")
			u, err := url.Parse(rawURL)
			if err != nil {
				return nil, fmt.Errorf("Unable to parse meta URL: %s", s)
			}
			r.URL = u
			continue
		}
		if strings.HasPrefix(s, "# nopm:draft") {
			r.Draft = true
			continue
		}
		metaDependsRegexp, err := regexp.Compile("# nopm:depends ([\\w-]+)")
		if err != nil {
			return nil, err
		}
		recipeRawNameMatch := metaDependsRegexp.FindStringSubmatch(s)
		if len(recipeRawNameMatch) == 2 {
			// we should handle circular dependencies here
			q := &RecipeQuery{
				Name: recipeRawNameMatch[1],
				e:    q.e,
			}
			d, err := q.Get()
			if err != nil {
				return nil, fmt.Errorf("Unable to read dependency %s", q.Name)
			}
			r.Depends[d.Name] = d
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
	var substVars []string
	scanner := bufio.NewScanner(strings.NewReader(string(r.Script)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		s := scanner.Text()

		if strings.HasPrefix(s, "# nopm:subst ") {
			_s := strings.TrimPrefix(s, "# nopm:subst ")
			newArg := strings.TrimSpace(_s)
			substVars = append(substVars, newArg)
		}

		for _, substVar := range substVars {
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
