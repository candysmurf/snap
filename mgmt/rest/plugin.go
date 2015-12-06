/*
http://www.apache.org/licenses/LICENSE-2.0.txt


Copyright 2015 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rest

import (
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/julienschmidt/httprouter"

	"github.com/intelsdi-x/snap/core"
	"github.com/intelsdi-x/snap/core/serror"
	"github.com/intelsdi-x/snap/mgmt/rest/rbody"
)

// PluginAlreadyLoaded is constant message
const PluginAlreadyLoaded = "plugin is already loaded"

var (
	// ErrMissingPluginName - error message for missing plugin name
	ErrMissingPluginName = errors.New("missing plugin name")
	// ErrPluginNotFound - error message for plugin not found
	ErrPluginNotFound    = errors.New("plugin not found")
)

type plugin struct {
	name       string
	version    int
	pluginType string
}

// Name returns the plugin name
func (p *plugin) Name() string {
	return p.name
}

// Version returns the plugin version
func (p *plugin) Version() int {
	return p.version
}

// TypeName returns the plugin type
func (p *plugin) TypeName() string {
	return p.pluginType
}

func (s *Server) loadPlugin(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		respond(500, rbody.FromError(err), w)
		return
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		var pluginPath string
		var signature []byte
		var checkSum [sha256.Size]byte
		lp := &rbody.PluginsLoaded{}
		lp.LoadedPlugins = make([]rbody.LoadedPlugin, 0)
		mr := multipart.NewReader(r.Body, params["boundary"])
		var i int
		for {
			var b []byte
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				respond(500, rbody.FromError(err), w)
				return
			}
			if r.Header.Get("Plugin-Compression") == "gzip" {
				g, err := gzip.NewReader(p)
				defer g.Close()
				if err != nil {
					respond(500, rbody.FromError(err), w)
					return
				}
				b, err = ioutil.ReadAll(g)
				if err != nil {
					respond(500, rbody.FromError(err), w)
					return
				}
			} else {
				b, err = ioutil.ReadAll(p)
				if err != nil {
					respond(500, rbody.FromError(err), w)
					return
				}
			}

			// A little sanity checking for files being passed into the API server.
			// First file passed in should be the plugin. If the first file is a signature
			// file, an error is returned. The signature file should be the second
			// file passed to the API server. If the second file does not have the ".asc"
			// extension, an error is returned.
			// If we loop around more than twice before receiving io.EOF, then
			// an error is returned.

			switch {
			case i == 0:
				if filepath.Ext(p.FileName()) == ".asc" {
					e := errors.New("Error: first file passed to load plugin api can not be signature file")
					respond(500, rbody.FromError(e), w)
					return
				}
				if pluginPath, err = writeFile(p.FileName(), b); err != nil {
					respond(500, rbody.FromError(err), w)
					return
				}
				checkSum = sha256.Sum256(b)
			case i == 1:
				if filepath.Ext(p.FileName()) == ".asc" {
					signature = b
				} else {
					e := errors.New("Error: second file passed was not a signature file")
					respond(500, rbody.FromError(e), w)
					return
				}
			case i == 2:
				e := errors.New("Error: More than two files passed to the load plugin api")
				respond(500, rbody.FromError(e), w)
				return
			}
			i++
		}
		rp, err := core.NewRequestedPlugin(pluginPath)
		if err != nil {
			respond(500, rbody.FromError(err), w)
			return
		}
		// Sanity check, verify the checkSum on the file sent is the same
		// as after it is written to disk.
		if rp.CheckSum() != checkSum {
			e := errors.New("Error: CheckSum mismatch on requested plugin to load")
			respond(500, rbody.FromError(e), w)
			return
		}
		rp.SetSignature(signature)
		restLogger.Info("Loading plugin: ", rp.Path())
		pl, err := s.mm.Load(rp)
		if err != nil {
			var ec int
			restLogger.Error(err)
			restLogger.Debugf("Removing file (%s)", rp.Path())
			err2 := os.RemoveAll(filepath.Dir(rp.Path()))
			if err2 != nil {
				restLogger.Error(err2)
			}
			rb := rbody.FromError(err)
			switch rb.ResponseBodyMessage() {
			case PluginAlreadyLoaded:
				ec = 409
			default:
				ec = 500
			}
			respond(ec, rb, w)
			return
		}
		lp.LoadedPlugins = append(lp.LoadedPlugins, *catalogedPluginToLoaded(r.Host, pl))
		respond(201, lp, w)
	}
}

func writeFile(filename string, b []byte) (string, error) {
	// Create temporary directory
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", err
	}
	f, err := os.Create(path.Join(dir, filename))
	if err != nil {
		return "", err
	}
	n, err := f.Write(b)
	log.Debugf("wrote %v to %v", n, f.Name())
	if err != nil {
		return "", err
	}
	err = f.Chmod(0700)
	if err != nil {
		return "", err
	}
	// Close before load
	f.Close()
	return f.Name(), nil
}

func (s *Server) unloadPlugin(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	plName := p.ByName("name")
	plType := p.ByName("type")
	plVersion, iErr := strconv.ParseInt(p.ByName("version"), 10, 0)
	f := map[string]interface{}{
		"plugin-name":    plName,
		"plugin-version": plVersion,
		"plugin-type":    plType,
	}

	if iErr != nil {
		se := serror.New(errors.New("invalid version"))
		se.SetFields(f)
		respond(400, rbody.FromSnapError(se), w)
		return
	}

	if plName == "" {
		se := serror.New(errors.New("missing plugin name"))
		se.SetFields(f)
		respond(400, rbody.FromSnapError(se), w)
		return
	}
	if plType == "" {
		se := serror.New(errors.New("missing plugin type"))
		se.SetFields(f)
		respond(400, rbody.FromSnapError(se), w)
		return
	}
	up, se := s.mm.Unload(&plugin{
		name:       plName,
		version:    int(plVersion),
		pluginType: plType,
	})
	if se != nil {
		se.SetFields(f)
		respond(500, rbody.FromSnapError(se), w)
		return
	}
	pr := &rbody.PluginUnloaded{
		Name:    up.Name(),
		Version: up.Version(),
		Type:    up.TypeName(),
	}
	respond(200, pr, w)
}

func (s *Server) getPlugins(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var detail bool
	// make this a function because DRY
	for k := range r.URL.Query() {
		if k == "details" {
			detail = true
		}
	}

	plugins := new(rbody.PluginList)

	// Cache the catalog here to avoid multiple reads
	plCatalog := s.mm.PluginCatalog()
	plugins.LoadedPlugins = make([]rbody.LoadedPlugin, len(plCatalog))
	for i, p := range s.mm.PluginCatalog() {
		plugins.LoadedPlugins[i] = *catalogedPluginToLoaded(r.Host, p)
	}

	if detail {
		aPlugins := s.mm.AvailablePlugins()
		plugins.AvailablePlugins = make([]rbody.AvailablePlugin, len(aPlugins))
		for i, p := range aPlugins {
			plugins.AvailablePlugins[i] = rbody.AvailablePlugin{
				Name:             p.Name(),
				Version:          p.Version(),
				Type:             p.TypeName(),
				HitCount:         p.HitCount(),
				LastHitTimestamp: p.LastHit().Unix(),
				ID:               p.ID(),
			}
		}
	}

	respond(200, plugins, w)
}

func catalogedPluginToLoaded(host string, c core.CatalogedPlugin) *rbody.LoadedPlugin {
	return &rbody.LoadedPlugin{
		Name:            c.Name(),
		Version:         c.Version(),
		Type:            c.TypeName(),
		Signed:          c.IsSigned(),
		Status:          c.Status(),
		LoadedTimestamp: c.LoadedTimestamp().Unix(),
		Href:            catalogedPluginURI(host, c),
	}
}

func (s *Server) getPluginsByType(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
}

func (s *Server) getPluginsByName(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
}

func (s *Server) getPlugin(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	plName := p.ByName("name")
	plType := p.ByName("type")
	plVersion, iErr := strconv.ParseInt(p.ByName("version"), 10, 0)
	f := map[string]interface{}{
		"plugin-name":    plName,
		"plugin-version": plVersion,
		"plugin-type":    plType,
	}

	if iErr != nil {
		se := serror.New(errors.New("invalid version"))
		se.SetFields(f)
		respond(400, rbody.FromSnapError(se), w)
		return
	}

	if plName == "" {
		se := serror.New(errors.New("missing plugin name"))
		se.SetFields(f)
		respond(400, rbody.FromSnapError(se), w)
		return
	}
	if plType == "" {
		se := serror.New(errors.New("missing plugin type"))
		se.SetFields(f)
		respond(400, rbody.FromSnapError(se), w)
		return
	}

	pluginCatalog := s.mm.PluginCatalog()
	var plugin core.CatalogedPlugin
	for _, item := range pluginCatalog {
		if item.Name() == plName &&
			item.Version() == int(plVersion) &&
			item.TypeName() == plType {
			plugin = item
			break
		}
	}
	if plugin == nil {
		se := serror.New(ErrPluginNotFound, f)
		respond(404, rbody.FromSnapError(se), w)
		return
	}

	rd := r.FormValue("download")
	d, _ := strconv.ParseBool(rd)
	if d {
		b, err := ioutil.ReadFile(plugin.PluginPath())
		if err != nil {
			f["plugin-path"] = plugin.PluginPath()
			se := serror.New(err, f)
			respond(500, rbody.FromSnapError(se), w)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		_, err = gz.Write(b)
		if err != nil {
			f["plugin-path"] = plugin.PluginPath()
			se := serror.New(err, f)
			respond(500, rbody.FromSnapError(se), w)
			return
		}
		return
	} 
	pluginRet := &rbody.PluginReturned{
		Name:            plugin.Name(),
		Version:         plugin.Version(),
		Type:            plugin.TypeName(),
		Signed:          plugin.IsSigned(),
		Status:          plugin.Status(),
		LoadedTimestamp: plugin.LoadedTimestamp().Unix(),
		Href:            catalogedPluginURI(r.Host, plugin),
	}
	respond(200, pluginRet, w)
}

func catalogedPluginURI(host string, c core.CatalogedPlugin) string {
	return fmt.Sprintf("%s://%s/v1/plugins/%s/%s/%d", protocolPrefix, host, c.TypeName(), c.Name(), c.Version())
}
