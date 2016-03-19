package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
)

type GifServlet struct {
	db              *sql.DB
	server_config   *Config
}

func NewGifServlet(server_config *Config) *GifServlet {
	t := new(GifServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewTruckServlet", "Failed to open database:", err)
	}
	t.db = db
	return t
}

func (t *GifServlet) Upload(r *http.Request) *ApiResult {
	// get gif
	// write to file
	// get your permissions right
	// return the URL
	gif_str := r.Form.Get("gif")
	log.Println(gif_str)
	return APISuccess("OK")

	// file_name, err := CreatePicFile(gif_str)
	// if err != nil {
	// 	log.Println(err)

	// }
	// return APISuccess("https://yaychakula.com/img/" + file_name)
}