package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"gopkg.in/russross/blackfriday.v2"

	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v28/github"
	"github.com/sirupsen/logrus"

	"golang.org/x/oauth2"
)

var (
	log           = logrus.New()
	listenAddr    string
	staticBaseURL string
	templatesDir  string
)

type RecipeQuery struct {
	Name    string
	Version string
}

type Recipe struct {
	Name    string
	Version string
	Script  []byte
	MoreMD  []byte
	MD      []byte
	Compat  map[string][]string
}

func GetFileContentType(out *os.File) (string, error) {

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	// Use the net/http package's handy DectectContentType function. Always returns a valid
	// content-type by returning "application/octet-stream" if no others seemed to match.
	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func renderMD(input []byte) string {
	unsafe := blackfriday.Run(input)
	html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
	return string(html)
}

func compatIconName(os string) string {
	switch os {
	case "linux":
		return "fl-tux"
	case "darwin":
		return "fl-apple"
	default:
		return ""
	}
}

func (q *RecipeQuery) Get() (*Recipe, error) {
	r := &Recipe{
		Name:    q.Name,
		Version: q.Version,
	}
	// TODO need security here
	data, err := ioutil.ReadFile(fmt.Sprintf("/Users/meister/recipes/%s.sh", r.Name))
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
		}
	}
	return r, nil
}

func All() ([]*Recipe, error) {
	recipesFilenames, err := filepath.Glob("/Users/meister/recipes/*.sh")
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

func isVersion(v string) bool {
	m := regexp.MustCompile(`^([0-9]+\.)*[0-9]*$`)
	return m.MatchString(v)
}

func (r *Recipe) loadMoreMD() error {
	data, err := ioutil.ReadFile(fmt.Sprintf("/Users/meister/recipes/%s.more.md", r.Name))
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

func ParseRecipeRawName(input string) *RecipeQuery {
	rawName := strings.Split(input, "@")
	name := rawName[0]
	version := ""
	if len(rawName) == 2 {
		version = rawName[1]
	}
	return &RecipeQuery{
		Name:    name,
		Version: version,
	}
}

// func indexHandler() http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		// defer func() {
// 		// 	log.WithFields(logrus.Fields{
// 		// 		"method":      r.Method,
// 		// 		"remote_addr": r.RemoteAddr,
// 		// 		"user_agent":  r.UserAgent(),
// 		// 	}).Info(r.URL.RequestURI())
// 		// }()
// 		recipeQuery := ParseRecipeURL(r.URL)
// 		recipe, err := recipeQuery.Get()
// 		if err != nil {
// 			http.Error(w, "No such recipe", http.StatusNotFound)
// 			return
// 		}
// 		render, err := recipe.Render()
// 		if err != nil {
// 			http.Error(w, err.Error(), http.StatusBadRequest)
// 			return
// 		}
// 		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
// 		w.Write(render)
// 	})
// }
//
// func tracing() func(http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			next.ServeHTTP(w, r)
//
// 		})
// 	}
// }
//
// func logging() func(http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			log.WithFields(logrus.Fields{
// 				"method":      r.Method,
// 				"remote_addr": r.RemoteAddr,
// 				"user_agent":  r.UserAgent(),
// 			}).Info(r.URL.RequestURI())
// 			next.ServeHTTP(w, r)
// 		})
// 	}
// }
//
// type loggingResponseWriter struct {
// 	status int
// 	body   string
// 	http.ResponseWriter
// }

func Base() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("staticBaseURL", staticBaseURL)
		c.Next()
	}
}

func loadTemplates(dir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()
	layouts, err := filepath.Glob(dir + "/layouts/*.tmpl")
	if err != nil {
		panic(err.Error())
	}

	includes, err := filepath.Glob(dir + "/includes/*.tmpl")
	if err != nil {
		panic(err.Error())
	}

	// Generate our templates map from our layouts/ and includes/ directories
	for _, include := range includes {
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(layoutCopy, include)
		r.AddFromFilesFuncs(filepath.Base(include), template.FuncMap{
			"renderMD":       renderMD,
			"compatIconName": compatIconName,
			"capitalize": func(s string) string {
				return strings.Title(s)
			},
			"htmlSafe": func(html string) template.HTML {
				return template.HTML(html)
			},
		},
			files...)
	}
	return r
}

func formatAsDate(t time.Time) string {
	year, month, day := t.Date()
	return fmt.Sprintf("%d%02d/%02d", year, month, day)
}

func main() {
	flag.StringVar(&listenAddr, "listen-addr", ":8080", "server listen address")
	flag.StringVar(&staticBaseURL, "static-base-url", "http://localhost:8081", "static files base URL")
	flag.StringVar(&templatesDir, "templates-dir", "./templates", "templates directory")
	flag.Parse()
	//
	// router := http.NewServeMux()
	// router.Handle("/", responseLogger(indexHandler()))
	//
	// // http.HandleFunc("/", indexHandler)
	// server := &http.Server{
	// 	Addr:    listenAddr,
	// 	Handler: tracing()(logging()(router)),
	// 	// ErrorLog:     logger,
	// 	ReadTimeout:  5 * time.Second,
	// 	WriteTimeout: 10 * time.Second,
	// 	IdleTimeout:  15 * time.Second,
	// }
	//
	// if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	// 	log.Fatalf("Could not listen on %s: %v\n", listenAddr, err)
	// }

	router := gin.Default()
	router.Use(gin.Recovery())
	router.Use(Base())

	router.HTMLRender = loadTemplates(templatesDir)
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"c":              c,
			"activeNavIndex": "active",
		})
	})
	router.GET("/docs", func(c *gin.Context) {
		c.HTML(http.StatusOK, "docs.tmpl", gin.H{
			"c":             c,
			"activeNavDocs": "active",
		})
	})
	router.GET("/security", func(c *gin.Context) {
		c.HTML(http.StatusOK, "security.tmpl", gin.H{
			"c":                 c,
			"activeNavSecurity": "active",
		})
	})
	router.GET("/recipes", func(c *gin.Context) {
		recipes, err := All()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "recipes.tmpl", gin.H{
			"c":                c,
			"activeNavRecipes": "active",
			"recipes":          recipes,
		})
	})
	router.GET("/recipes/:recipe", func(c *gin.Context) {
		recipeQuery := ParseRecipeRawName(c.Param("recipe"))
		recipe, err := recipeQuery.Get()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "recipe.tmpl", gin.H{
			"c":                c,
			"activeNavRecipes": "active",
			"recipe":           recipe,
		})
	})
	router.GET("/download_url/github.com/:owner/:repo/releases/:version/:os/:arch/:ext", func(c *gin.Context) {
		owner := c.Param("owner")
		redirect := c.Query("redirect")
		repo := c.Param("repo")
		arch := strings.ToLower(c.Param("arch"))
		os := strings.ToLower(c.Param("os"))
		ext := strings.ToLower(c.Param("ext"))
		// os := c.Param("repo")
		// ext := c.Param("ext")
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: "67320610138359eb4487b6796becfc837cf9a0e0"},
		)
		tc := oauth2.NewClient(ctx, ts)
		var rID int64
		client := github.NewClient(tc)
		releaseID, _, errGetLatest := client.Repositories.GetLatestRelease(ctx, owner, repo)
		if errGetLatest != nil {
			releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, nil)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			if len(releases) == 0 {
				c.String(http.StatusInternalServerError, errGetLatest.Error())
				return
			}
			rID = *releases[0].ID
		} else {
			rID = *releaseID.ID
		}

		assets, _, err := client.Repositories.ListReleaseAssets(ctx, owner, repo, rID, nil)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		var downloadURL string
		if len(assets) == 0 {
			c.String(http.StatusNotFound, "No assets found in this release")
			return
		}
		for _, asset := range assets {
			url := strings.ToLower(*asset.BrowserDownloadURL)
			if !strings.Contains(url, arch) {
				continue
			}
			if !strings.Contains(url, os) {
				continue
			}
			if !strings.HasSuffix(url, ext) {
				continue
			}
			downloadURL = *asset.BrowserDownloadURL
		}
		if downloadURL == "" {
			c.String(http.StatusNotFound, "No assets found in this release with current criteras")
			return
		}
		if redirect == "true" {
			c.Redirect(http.StatusFound, downloadURL)
		} else {
			c.String(http.StatusOK, downloadURL)
		}

	})
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method
		fmt.Println(path)
		fmt.Println(method)
		if strings.HasPrefix(path, "/show") {
			fmt.Println("ok")
		}
	})
	router.GET("/search/:input", func(c *gin.Context) {
		input := strings.ToLower(c.Param("input"))
		recipes, err := All()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		var results []string
		for _, recipe := range recipes {
			if strings.Contains(strings.ToLower(recipe.Name), input) {
				results = append(results, recipe.Name)
			}
		}

		c.String(http.StatusOK, strings.Join(results, "\n"))
	})

	router.GET("/md/:recipeName/more", func(c *gin.Context) {
		recipeName := c.Param("recipeName")
		recipeQuery := ParseRecipeRawName(recipeName)
		recipe, err := recipeQuery.Get()
		if err != nil {
			c.String(http.StatusNotFound, "Recipe not found")
			return
		}
		if recipe.MoreMD == nil {
			c.String(http.StatusNotFound, "Recipe additional documentation not found")
			return
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", recipe.MoreMD)
	})
	// router.GET("/:eroute", func(c *gin.Context) {
	// 	c.String(http.StatusOK, c.Param("route"))
	// })
	// router.GET("/terraform/toto", func(c *gin.Context) {
	// 	c.String(http.StatusOK, c.Param("route"))
	// })
	//
	// router.GET("/:recipeRawName", func(c *gin.Context) {
	//
	// 	c.String(http.StatusOK, c.Param("recipeRawName"))
	// 	return
	// 	// fmt.Println("coucou", c.Param("recipeRawName"))
	// 	// if c.Param("recipeRawName") == "_" {
	// 	// 	c.String(http.StatusOK, "cool")
	// 	// 	return
	// 	// }
	// 	// recipeRawName := c.Param("recipeRawName")
	// 	//
	// 	// recipeQuery := ParseRecipeRawName(recipeRawName)
	// 	// recipe, err := recipeQuery.Get()
	// 	// if err != nil {
	// 	// 	c.String(http.StatusNotFound, "Recipe not found")
	// 	// 	return
	// 	// }
	// 	// render, err := recipe.Render()
	// 	// if err != nil {
	// 	// 	c.String(http.StatusBadRequest, err.Error())
	// 	// 	return
	// 	// }
	// 	// c.String(http.StatusOK, string(render))
	// })

	router.Run(listenAddr)
}
