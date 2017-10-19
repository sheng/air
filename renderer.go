package air

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// renderer is used to provide a `Render()` method for an `Air` instance
// for renders a "text/html" HTTP response.
type renderer struct {
	air *Air

	template        *template.Template
	templateFuncMap template.FuncMap
	watcher         *fsnotify.Watcher
}

// newRenderer returns a pointer of a new instance of the `renderer`.
func newRenderer(a *Air) *renderer {
	return &renderer{
		air:      a,
		template: template.New("template"),
		templateFuncMap: template.FuncMap{
			"strlen":  strlen,
			"strcat":  strcat,
			"substr":  substr,
			"timefmt": timefmt,
		},
	}
}

// init initializes the `Renderer`. It will be called in the `Air#Serve()`.
//
// e.g. r.air.Config.TemplateRoot == "templates" && r.air.Config.TemplateExt ==
// []string{".html"}
//
// templates/
//   index.html
//   login.html
//   register.html
//
// templates/parts/
//   header.html
//   footer.html
//
// will be parsed into:
//
// "index.html", "login.html", "register.html", "parts/header.html",
// "parts/footer.html".
func (r *renderer) init() error {
	if _, err := os.Stat(r.air.TemplateRoot); os.IsNotExist(err) {
		return nil
	}

	tr, err := filepath.Abs(r.air.TemplateRoot)
	if err != nil {
		return err
	}

	dirs, files, err := walkFiles(tr, r.air.TemplateExts)
	if err != nil {
		return err
	}

	if r.watcher == nil {
		if r.watcher, err = fsnotify.NewWatcher(); err != nil {
			return err
		}

		for _, dir := range dirs {
			if err := r.watcher.Add(dir); err != nil {
				return err
			}
		}

		go r.watchTemplates()
	}

	t := template.New("template")
	t.Funcs(r.templateFuncMap)
	t.Delims(r.air.TemplateLeftDelim, r.air.TemplateRightDelim)

	for _, file := range files {
		b, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}

		name := filepath.ToSlash(file[len(tr)+1:])
		if _, err = t.New(name).Parse(string(b)); err != nil {
			return err
		}
	}

	r.template = t

	return nil
}

// setTemplateFunc sets the func f into the `Renderer` with the
// name.
func (r *renderer) setTemplateFunc(name string, f interface{}) {
	r.templateFuncMap[name] = f
}

// render renders the data into the w with the templateName.
func (r *renderer) render(w io.Writer, templateName string, data map[string]interface{}) error {
	return r.template.ExecuteTemplate(w, templateName, data)
}

// watchTemplates watchs the changing of all template files.
func (r *renderer) watchTemplates() {
	for {
		select {
		case event := <-r.watcher.Events:
			r.air.Logger.Info(event)

			if event.Op == fsnotify.Create {
				r.watcher.Add(event.Name)
			}

			if err := r.init(); err != nil {
				r.air.Logger.Error(err)
			}
		case err := <-r.watcher.Errors:
			r.air.Logger.Error(err)
		}
	}
}

// walkFiles walks all files with the exts in all subdirs of the root
// recursively.
func walkFiles(
	root string,
	exts []string,
) (
	dirs []string,
	files []string,
	err error,
) {
	if err = filepath.Walk(
		root,
		func(path string, info os.FileInfo, err error) error {
			if info != nil && info.IsDir() {
				dirs = append(dirs, path)
			}
			return err
		},
	); err != nil {
		return nil, nil, err
	}

	for _, dir := range dirs {
		for _, ext := range exts {
			fs, err := filepath.Glob(filepath.Join(dir, "*"+ext))
			if err != nil {
				return nil, nil, err
			}
			files = append(files, fs...)
		}
	}

	return
}

// strlen returns the number of chars in the s.
func strlen(s string) int {
	return len([]rune(s))
}

// strcat returns a string that is catenated to the tail of the s by the ss.
func strcat(s string, ss ...string) string {
	for i := range ss {
		s = fmt.Sprintf("%s%s", s, ss[i])
	}
	return s
}

// substr returns the substring consisting of the chars of the s starting at the
// index i and continuing up to, but not including, the char at the index j.
func substr(s string, i, j int) string {
	rs := []rune(s)
	return string(rs[i:j])
}

// timefmt returns a textual representation of the t formatted according to the
// layout.
func timefmt(t time.Time, layout string) string {
	return t.Format(layout)
}
