package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nopm-sh/nopm-sh/core"

	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v28/github"
	"github.com/sirupsen/logrus"

	"github.com/go-redis/redis/v7"
	"golang.org/x/oauth2"
)

var (
	log           = logrus.New()
	listenAddr    string
	staticBaseURL string
	templatesDir  string
	recipesDir    string
	version       string
	showVersion   bool
	redisAddr     string
	redisClient   *redis.Client
	redisDB       int
	recipesEngine *core.RecipeEngine
)

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
			"renderMD":       core.RenderMD,
			"compatIconName": compatIconName,
			"capitalize": func(s string) string {
				return strings.Title(s)
			},
			"plural": func(i int) string {
				if i == 0 || i > 1 {
					return "s"
				}
				return ""
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

	redisAddr = os.Getenv("REDIS_ADDR")

	flag.StringVar(&listenAddr, "listen-addr", ":8080", "server listen address")
	flag.StringVar(&redisAddr, "redis-addr", "", "Redis server address")
	flag.IntVar(&redisDB, "redis-db", 0, "Redis database")
	flag.StringVar(&staticBaseURL, "static-base-url", "", "static files base URL")
	flag.StringVar(&templatesDir, "templates-dir", "./templates", "templates directory")
	flag.StringVar(&recipesDir, "recipes-dir", "", "recipes directory")
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.Parse()

	if showVersion {
		fmt.Printf("nopm-sh version %s", version)
		os.Exit(0)
	}

	if staticBaseURL == "" {
		staticBaseURL = os.Getenv("STATIC_BASE_URL")
	}

	if staticBaseURL == "" {
		staticBaseURL = "./templates"
	}

	if redisAddr == "" {
		redisAddr = os.Getenv("REDIS_ADDR")
	}

	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if recipesDir == "" {
		recipesDir = os.Getenv("RECIPES_DIR")
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable empty")
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       redisDB,
	})

	recipesEngine = core.NewRecipeEngine(redisClient, recipesDir)

	router := gin.Default()
	router.Use(gin.Recovery())
	router.Use(Base())

	recipeRegexp, err := regexp.Compile("^/([^/]+)$")
	if err != nil {
		log.Fatal(err)
	}

	router.HTMLRender = loadTemplates(templatesDir)
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"c":              c,
			"activeNavIndex": "active",
		})
	})
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": version})
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
		recipes, err := recipesEngine.All()
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
		recipeQuery := recipesEngine.ParseRecipeRawName(c.Param("recipe"))
		recipe, err := recipeQuery.Get()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "recipe_readme.tmpl", gin.H{
			"c":                c,
			"activeNavRecipes": "active",
			"recipe":           recipe,
			"activeTabReadme":  "active",
		})
	})

	router.GET("/recipes/:recipe/source", func(c *gin.Context) {
		recipeQuery := recipesEngine.ParseRecipeRawName(c.Param("recipe"))
		recipe, err := recipeQuery.Get()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "recipe_source.tmpl", gin.H{
			"c":                c,
			"activeNavRecipes": "active",
			"recipe":           recipe,
			"source":           string(recipe.Script),
			"activeTabSource":  "active",
		})
	})
	router.GET("/recipes/:recipe/meta", func(c *gin.Context) {
		recipeQuery := recipesEngine.ParseRecipeRawName(c.Param("recipe"))
		recipe, err := recipeQuery.Get()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "recipe_meta.tmpl", gin.H{
			"c":                c,
			"activeNavRecipes": "active",
			"recipe":           recipe,
			"activeTabMeta":    "active",
		})
	})
	router.GET("/recipes/:recipe/dependencies", func(c *gin.Context) {
		recipeQuery := recipesEngine.ParseRecipeRawName(c.Param("recipe"))
		recipe, err := recipeQuery.Get()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "recipe_dependencies.tmpl", gin.H{
			"c":                     c,
			"recipe":                recipe,
			"activeNavRecipes":      "active",
			"activeTabDependencies": "active",
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
			&oauth2.Token{AccessToken: githubToken},
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
		switch method {
		case "HEAD":
			recipeRawNameMatch := recipeRegexp.FindStringSubmatch(path)
			if len(recipeRawNameMatch) == 2 {
				recipeQuery := recipesEngine.ParseRecipeRawName(recipeRawNameMatch[1])
				_, err := recipeQuery.Get()
				if err != nil {
					c.String(http.StatusNotFound, "Recipe not found")
					return
				}
				if method == "HEAD" {
					c.String(http.StatusFound, "")
					return
				}
			}
		case "GET":
			recipeRawNameMatch := recipeRegexp.FindStringSubmatch(path)
			if len(recipeRawNameMatch) == 2 {
				recipeQuery := recipesEngine.ParseRecipeRawName(recipeRawNameMatch[1])
				recipe, err := recipeQuery.Get()
				if err != nil {
					c.String(http.StatusNotFound, "Recipe not found")
					return
				}
				if method == "HEAD" {
					c.String(http.StatusFound, "Recipe exists")
					return
				}
				redisClient.Incr(fmt.Sprintf("%s_hits", recipe.Name))
				render, err := recipe.Render()
				if err != nil {
					c.String(http.StatusBadRequest, err.Error())
					return
				}
				c.String(http.StatusOK, string(render))
			}
		}

	})

	router.GET("/search/:input", func(c *gin.Context) {
		input := strings.ToLower(c.Param("input"))
		results, err := recipesEngine.Search(input)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.String(http.StatusOK, strings.Join(results, "\n"))
	})

	router.GET("/md/:recipeName/more", func(c *gin.Context) {
		recipeName := c.Param("recipeName")
		recipeQuery := recipesEngine.ParseRecipeRawName(recipeName)
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

	router.Run(listenAddr)
}
