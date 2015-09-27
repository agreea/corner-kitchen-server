package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"strings"
)

type TemplateFunc func(*TemplateHelpers, *http.Request) (interface{}, error)

type TemplateHelpers struct {
	db      *sql.DB
	twilio  chan *SMS
	session *SessionManager
}

type TemplateHandler struct {
	TemplateFuncs map[string]TemplateFunc
	TemplateFiles map[string]string
	Helpers       *TemplateHelpers
	Config        *Config
}

func NewTemplateHandler(server_config *Config, db *sql.DB, twilio chan *SMS, session *SessionManager) *TemplateHandler {
	h := new(TemplateHandler)
	h.TemplateFuncs = make(map[string]TemplateFunc)
	h.TemplateFiles = make(map[string]string)
	h.Config = server_config
	h.Helpers = new(TemplateHelpers)
	h.Helpers.db = db
	h.Helpers.twilio = twilio
	h.Helpers.session = session
	return h
}

func (t *TemplateHandler) HandleTemplate(endpoint string, handler TemplateFunc, template string) {
	t.TemplateFuncs[endpoint] = handler
	t.TemplateFiles[endpoint] = template
}

func (t *TemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	template_name := r.URL.Path[1:]
	if query_idx := strings.Index(template_name, "?"); query_idx != -1 {
		template_name = template_name[:query_idx]
	}

	if template_func, template_exists := t.TemplateFuncs[template_name]; template_exists {
		r.ParseForm()

		template_data, err := template_func(t.Helpers, r)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", 500)
			return
		}

		template_path := path.Join(t.Config.Templates.Root, t.TemplateFiles[template_name])
		template, err := template.ParseFiles(template_path)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", 500)
			return
		}

		err = template.Execute(w, template_data)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", 500)
			return
		}

	} else {
		http.Error(w, fmt.Sprintf("Handler '%s' not found", template_name), 404)
	}
}
