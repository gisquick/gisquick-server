package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gisquick/gisquick-server/internal/infrastructure/cache"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type AliasManager struct {
	server       *Server
	configReader *cache.JSONFileReader[map[string]string]
}

func saveJsonFile(path string, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	if err := encoder.Encode(data); err != nil {
		return err
	}
	return nil
}

func configFilename(c echo.Context) string {
	domain := c.QueryParam("domain")
	filename := "default"
	if domain != "" {
		filename = domain
	}
	return filepath.Join("/etc/gisquick/aliases", filename+".json")
}

func (a *AliasManager) projectExists(name string) bool {
	path := filepath.Join(a.server.Config.ProjectsRoot, name)
	_, err := os.Stat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func (a *AliasManager) handleGetAliases(c echo.Context) error {
	filename := configFilename(c)
	names, err := a.server.projects.ProjectsNames(false)
	if err != nil {
		return err
	}
	aliases, err := a.configReader.Get(filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data := make(map[string]string, len(names))
	for _, name := range names {
		data[name] = ""
	}
	for alias, name := range aliases {
		data[name] = alias
	}
	return c.JSON(http.StatusOK, data)
}

func (a *AliasManager) handleSetProjectAlias() func(c echo.Context) error {
	type Form struct {
		Alias       string `json:"alias"`
		ProjectName string `json:"name" validate:"required"`
	}
	var validate = validator.New()
	return func(c echo.Context) error {
		form := new(Form)
		if err := (&echo.DefaultBinder{}).BindBody(c, &form); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		// if err := c.Bind(form); err != nil {
		// 	return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		// }
		if err := validate.Struct(form); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		filename := configFilename(c)
		aliases, err := a.configReader.Get(filename)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			aliases = make(map[string]string, 1)
		}
		projectName, exists := aliases[form.Alias]
		if exists && a.projectExists(projectName) {
			return echo.NewHTTPError(http.StatusConflict, "Alias already exists")
		}
		// remove old alias and obsolete records
		for alias, name := range aliases {
			if name == form.ProjectName || !a.projectExists(name) {
				delete(aliases, alias)
			}
		}
		if form.Alias != "" {
			aliases[form.Alias] = form.ProjectName
		}
		if err = saveJsonFile(filename, aliases); err != nil {
			return err
		}
		return a.handleGetAliases(c)
	}
}

func (a *AliasManager) handleGetProjectName() func(c echo.Context) error {
	return func(c echo.Context) error {
		name := c.Param("name")
		aliases, err := a.configReader.Get(configFilename(c))
		if err != nil {
			a.server.log.Warnw("handleGetProject", zap.Error(err))
		} else {
			name = aliases[name]
			if name != "" {
				req := c.Request().Clone(c.Request().Context())
				req.URL.Path = "/api/map/project/" + name
				a.server.echo.ServeHTTP(c.Response(), req)
				return nil
			}
		}
		return echo.NewHTTPError(http.StatusBadRequest, "Project does not exists")
	}
}

func AddAliasAPI(s *Server) {
	aliasesReader := cache.NewJSONFileReader[map[string]string](24 * time.Hour)
	s.OnShutdown(aliasesReader.Close)
	am := &AliasManager{
		server:       s,
		configReader: aliasesReader,
	}
	s.echo.GET("/api/admin/aliases", am.handleGetAliases, s.middlewares.SuperuserRequired)
	s.echo.POST("/api/admin/alias", am.handleSetProjectAlias(), s.middlewares.SuperuserRequired)
	s.echo.GET("/api/map/alias/:name", am.handleGetProjectName())
}
